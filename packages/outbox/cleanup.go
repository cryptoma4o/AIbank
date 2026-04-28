package outbox

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// DefaultCleanupBatchSize — сколько строк удаляем за один DELETE-запрос.
// 1000 — компромисс: достаточно крупно, чтобы покрыть типовой суточный объём
// billing-events за единицы итераций, но не настолько большой, чтобы на
// гигантских таблицах удержать tx-lock дольше пары секунд (важно: relay в
// это время может пытаться SELECT FOR UPDATE SKIP LOCKED — на published-rows
// он не пересекается с DELETE, но short transactions всё равно безопаснее).
const DefaultCleanupBatchSize = 1000

// DefaultCleanupRetention — сколько держим published-строки после публикации.
// 7 дней — стандартный «достаточно для forensic / replay, но без бесконечного
// роста». Значение зеркалируется в TODO README.md и в Helm CronJob defaults.
const DefaultCleanupRetention = 7 * 24 * time.Hour

// CleanupPublished итеративно удаляет уже опубликованные строки старше
// retention из outbox-таблицы. Возвращает суммарное число удалённых строк.
//
// Реализация:
//   - Используется DELETE ... WHERE id IN (SELECT id ... LIMIT batchSize
//     FOR UPDATE SKIP LOCKED). SKIP LOCKED делает cleanup безопасным для
//     активного relay'я (relay блокирует unpublished-строки; cleanup —
//     published; пересечений быть не может, но SKIP LOCKED страхует от
//     любых параллельных DELETE/UPDATE).
//   - Цикл повторяется, пока DELETE возвращает ровно batchSize строк, что
//     означает «возможно, осталось ещё». На пустой пачке выходим.
//   - Каждая итерация — отдельная транзакция (autocommit), чтобы не держать
//     блокировки на гигантских таблицах и не раздувать WAL единым DELETE.
//
// Идемпотентен: повторный вызов после успешного завершения вернёт 0.
//
// retention < 0 → ошибка валидации (защита от случайного DELETE всех строк).
// retention == 0 допустим только для тестов: удалит все published-строки
// независимо от давности (use with care).
//
// batchSize <= 0 → DefaultCleanupBatchSize.
func (o *Outbox) CleanupPublished(
	ctx context.Context,
	retention time.Duration,
	batchSize int,
) (int, error) {
	if o == nil {
		return 0, errors.New("outbox: receiver is nil")
	}
	if retention < 0 {
		return 0, fmt.Errorf("outbox: retention must be >= 0, got %s", retention)
	}
	if batchSize <= 0 {
		batchSize = DefaultCleanupBatchSize
	}

	// retention выражаем в секундах в виде literal-интервала, чтобы не
	// гонять interval-параметр через драйвер (не все драйверы умеют
	// привязывать time.Duration к INTERVAL). Альтернатива — NOW() - $1::interval,
	// но с числовым literal проще и быстрее в plan'е.
	retentionSeconds := int64(retention / time.Second)

	// Sub-SELECT с FOR UPDATE SKIP LOCKED гарантирует, что:
	//  * если relay (или другой cleanup-runner) уже залочил строку — мы её
	//    пропустим, а не будем ждать;
	//  * на больших таблицах DELETE ограничен LIMIT'ом и не превратится в
	//    full-table scan-and-lock.
	deleteSQL := fmt.Sprintf(
		`DELETE FROM %s WHERE id IN (
			SELECT id FROM %s
			WHERE published_at IS NOT NULL
			  AND published_at < NOW() - make_interval(secs => %d)
			ORDER BY id
			LIMIT %d
			FOR UPDATE SKIP LOCKED
		)`,
		o.table, o.table, retentionSeconds, batchSize)

	totalDeleted := 0
	for {
		// Уважаем отмену контекста между итерациями (cron в k8s может его
		// дёрнуть при graceful shutdown).
		if err := ctx.Err(); err != nil {
			return totalDeleted, err
		}

		res, err := o.db.ExecContext(ctx, deleteSQL)
		if err != nil {
			return totalDeleted, fmt.Errorf("outbox cleanup: delete: %w", err)
		}
		affected, err := res.RowsAffected()
		if err != nil {
			return totalDeleted, fmt.Errorf("outbox cleanup: rows affected: %w", err)
		}
		totalDeleted += int(affected)

		// Меньше batchSize → больше нечего удалять, выходим.
		if affected < int64(batchSize) {
			return totalDeleted, nil
		}
	}
}
