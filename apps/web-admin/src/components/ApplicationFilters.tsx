"use client";

// ApplicationFilters — sidebar с чекбоксами по состояниям, поиском и
// диапазоном дат.  Состояние фильтров — controlled, родитель сам решает,
// как применять (передаёт в applications query как ApplicationsFilter и
// дополнительно фильтрует на клиенте по INN/датам, потому что bff-admin
// не имеет таких полей в схеме на v0.1).

import { APPLICATION_STATES, stateLabel } from "@/lib/application-states";

export interface FiltersValue {
  /** Множество выбранных состояний; пусто = все. */
  states: Set<string>;
  /** Поисковая строка по INN (12 цифр для физлица, 10 для юрлица). */
  query: string;
  /** Дата создания «с» в формате YYYY-MM-DD. */
  dateFrom: string;
  /** Дата создания «по» в формате YYYY-MM-DD. */
  dateTo: string;
}

export const EMPTY_FILTERS: FiltersValue = {
  states: new Set(),
  query: "",
  dateFrom: "",
  dateTo: "",
};

interface Props {
  value: FiltersValue;
  onChange: (next: FiltersValue) => void;
  onReset: () => void;
}

export function ApplicationFilters({ value, onChange, onReset }: Props) {
  function toggleState(state: string) {
    const next = new Set(value.states);
    if (next.has(state)) next.delete(state);
    else next.add(state);
    onChange({ ...value, states: next });
  }

  return (
    <aside className="sticky top-4 w-64 shrink-0 rounded-md border border-gray-200 bg-white p-4 text-sm">
      <div className="flex items-center justify-between">
        <h2 className="font-semibold text-gray-900">Фильтры</h2>
        <button
          type="button"
          onClick={onReset}
          className="text-xs text-primary hover:underline"
        >
          Сбросить
        </button>
      </div>

      <div className="mt-4">
        <label
          htmlFor="filter-query"
          className="mb-1 block text-xs font-medium text-gray-700"
        >
          Поиск по ИНН
        </label>
        <input
          id="filter-query"
          type="text"
          inputMode="numeric"
          placeholder="7707083893"
          value={value.query}
          onChange={(e) => onChange({ ...value, query: e.target.value })}
          className="w-full rounded-md border border-gray-300 px-2.5 py-1.5 text-sm shadow-sm focus:border-primary"
        />
      </div>

      <div className="mt-4 grid grid-cols-2 gap-2">
        <div>
          <label
            htmlFor="filter-date-from"
            className="mb-1 block text-xs font-medium text-gray-700"
          >
            Дата с
          </label>
          <input
            id="filter-date-from"
            type="date"
            value={value.dateFrom}
            onChange={(e) =>
              onChange({ ...value, dateFrom: e.target.value })
            }
            className="w-full rounded-md border border-gray-300 px-2 py-1 text-xs"
          />
        </div>
        <div>
          <label
            htmlFor="filter-date-to"
            className="mb-1 block text-xs font-medium text-gray-700"
          >
            Дата по
          </label>
          <input
            id="filter-date-to"
            type="date"
            value={value.dateTo}
            onChange={(e) => onChange({ ...value, dateTo: e.target.value })}
            className="w-full rounded-md border border-gray-300 px-2 py-1 text-xs"
          />
        </div>
      </div>

      <fieldset className="mt-5">
        <legend className="mb-2 text-xs font-medium uppercase tracking-wider text-gray-500">
          Состояния
        </legend>
        <div className="max-h-72 space-y-1.5 overflow-y-auto pr-1">
          {APPLICATION_STATES.map((s) => (
            <label
              key={s}
              className="flex items-center gap-2 text-sm text-gray-700"
            >
              <input
                type="checkbox"
                checked={value.states.has(s)}
                onChange={() => toggleState(s)}
                className="h-4 w-4 rounded border-gray-300 text-primary focus:ring-primary"
              />
              <span>{stateLabel(s)}</span>
            </label>
          ))}
        </div>
      </fieldset>
    </aside>
  );
}
