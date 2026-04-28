// graphql-operations.ts — все запросы и мутации к bff-admin.
//
// Имена и форма полей точно соответствуют schema.graphqls
// (services/bff-admin/graph/schema.graphqls).  При изменении контракта
// правки нужны и здесь, и в graph/resolver.go.

import { gql } from "@apollo/client";

export const QUERY_TENANT = gql`
  query AdminTenant {
    tenant {
      id
      name
      bik
      inn
      status
      deploymentMode
    }
  }
`;

export const QUERY_APPLICATIONS = gql`
  query AdminApplications($filter: ApplicationsFilter) {
    applications(filter: $filter) {
      id
      tenantId
      applicantId
      state
      legalEntityType
      channel
      productCodes
      createdAt
      updatedAt
      riskAssessment {
        id
        score
        category
        recommendation
        computedAt
      }
      decision {
        id
        decision
        reasoning
        decidedAt
      }
    }
  }
`;

export const QUERY_APPLICATION = gql`
  query AdminApplication($id: ID!) {
    application(id: $id) {
      id
      tenantId
      applicantId
      state
      legalEntityType
      channel
      productCodes
      createdAt
      updatedAt
      riskAssessment {
        id
        applicationId
        score
        category
        recommendation
        computedAt
        factors
      }
      decision {
        id
        applicationId
        decision
        reasoning
        decidedAt
      }
    }
  }
`;

export const QUERY_AUDIT_EVENTS = gql`
  query AdminAuditEvents($filter: AuditFilter) {
    auditEvents(filter: $filter) {
      id
      tenantId
      actorType
      actorId
      action
      subjectType
      subjectId
      occurredAt
      data
    }
  }
`;

export const QUERY_TENANT_CONFIG = gql`
  query AdminTenantConfig {
    tenantConfig {
      tenantId
      schemaVersion
      rawConfig
      updatedAt
    }
  }
`;

export const MUTATION_UPDATE_DECISION = gql`
  mutation UpdateApplicationDecision($input: UpdateApplicationDecisionInput!) {
    updateApplicationDecision(input: $input) {
      id
      applicationId
      decision
      reasoning
      decidedAt
    }
  }
`;
