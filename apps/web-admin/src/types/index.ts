export interface Application {
  id: string;
  tenant_id: string;
  status: string;
  inn: string;
  ogrn: string;
}

export interface AuditEvent {
  id: string;
  event_type: string;
  actor_id: string;
  resource_id: string;
  occurred_at: string;
}

export interface Tenant {
  id: string;
  name: string;
  slug: string;
}

export interface UBONode {
  id: string;
  name: string;
  node_type: string;
  stake: number;
  is_ubo: boolean;
}

export interface UBOGraph {
  app_id: string;
  nodes: UBONode[];
  ubos: UBONode[];
}
