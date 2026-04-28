// roles.ts — список ролей, которые имеют право входить в admin-панель.
//
// Карта ролей зеркалит identity-service domain.Role:
//   platform.admin — операционная команда AIbank, полный доступ ко всем
//     тенантам (только она может suspendTenant и редактировать tenant.yaml
//     через GitOps; в admin-панели — read-only view).
//   bank.admin — администратор банка, видит ВСЁ внутри своего тенанта,
//     включая audit log и tenant config.
//   bank.compliance_officer — комплаенс-офицер: читает заявки и решает
//     manual_review.
//   bank.operator — оператор первой линии: видит заявки и помогает с
//     `requires_more_info`, не может одобрить/отказать.
//
// Любая другая роль (`applicant`, инструменты внешних интеграций) —
// попадает на блок-экран при попытке войти.

export const ADMIN_ROLES = [
  "platform.admin",
  "bank.admin",
  "bank.compliance_officer",
  "bank.operator",
] as const;

export type AdminRole = (typeof ADMIN_ROLES)[number];

/** Роли, которые могут принимать decision (approve/decline/edd). */
export const DECISION_ROLES: readonly string[] = [
  "platform.admin",
  "bank.admin",
  "bank.compliance_officer",
];

/** Роли, которые видят audit log целиком. */
export const AUDIT_ROLES: readonly string[] = [
  "platform.admin",
  "bank.admin",
];

/** Роли, которые видят tenant config. */
export const TENANT_CONFIG_ROLES: readonly string[] = [
  "platform.admin",
  "bank.admin",
];

const ROLE_LABELS: Record<string, string> = {
  "platform.admin": "Администратор платформы",
  "bank.admin": "Администратор банка",
  "bank.compliance_officer": "Комплаенс-офицер",
  "bank.operator": "Оператор",
  applicant: "Клиент",
};

export function roleLabel(role: string): string {
  return ROLE_LABELS[role] ?? role;
}
