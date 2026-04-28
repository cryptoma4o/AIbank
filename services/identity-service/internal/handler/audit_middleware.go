package handler

import (
	"context"
	"log/slog"
	"net/http"

	auditsdk "github.com/aibank/platform/packages/audit-sdk"
)

// auditCtxKey — ключ context для мутабельного holder'а.  Holder
// заполняется handler'ом ПОСЛЕ парсинга тела/токена, поэтому простая
// AuthInfo, выставленная в middleware, не сработала бы (tenant_id и
// actor_id не известны на входе).
type auditCtxKey struct{}

// auditCtx — holder с данными для аудит-события.  ActorType пустой =
// auditsdk.ActorTypeSystem (см. middleware EmitOnSuccess: при невалидном
// типе он подставит system).
type auditCtx struct {
	tenantID  string
	actorID   string
	actorType auditsdk.ActorType
}

// withAuditHolder кладёт пустой holder в context — handler заполняет его
// через rememberActor/rememberTenantID.
func withAuditHolder(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		holder := &auditCtx{}
		ctx := context.WithValue(r.Context(), auditCtxKey{}, holder)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// rememberActor записывает actor_id, actor_type и tenant_id в holder и
// синхронизирует tenant_id с auditsdk.SetTenantID, чтобы EmitOnSuccess
// middleware видел корректные значения после возврата handler'а.
func rememberActor(r *http.Request, tenantID, actorID string, actorType auditsdk.ActorType) {
	if h, ok := r.Context().Value(auditCtxKey{}).(*auditCtx); ok {
		h.tenantID = tenantID
		h.actorID = actorID
		h.actorType = actorType
	}
	auditsdk.SetTenantID(r.Context(), tenantID)
}

// emitOnSuccessFromHolder — wraps auditsdk.EmitOnSuccess: до handler'а
// выставляет AuthInfo из holder (актёрский фолбэк "system"); handler через
// rememberActor может перезатереть holder, но AuthInfo нужен с самого
// начала, иначе middleware bail'ит ещё до резолвинга entity.
func emitOnSuccessFromHolder(client *auditsdk.Client, eventType string,
	resolve auditsdk.EntityResolver, log *slog.Logger,
) func(http.Handler) http.Handler {
	emit := auditsdk.EmitOnSuccess(client, eventType, resolve, log)
	return func(next http.Handler) http.Handler {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if h, ok := r.Context().Value(auditCtxKey{}).(*auditCtx); ok {
					actorID := h.actorID
					if actorID == "" {
						actorID = "system"
					}
					actorType := h.actorType
					if !actorType.IsValid() {
						actorType = auditsdk.ActorTypeSystem
					}
					ctx := auditsdk.WithAuthInfo(r.Context(), auditsdk.AuthInfo{
						TenantID:  h.tenantID,
						ActorID:   actorID,
						ActorType: actorType,
					})
					r = r.WithContext(ctx)
				}
				next.ServeHTTP(w, r)
			})
		}(emit(next))
	}
}
