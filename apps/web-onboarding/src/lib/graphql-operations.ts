// graphql-operations.ts — все запросы и мутации к bff-onboarding.
//
// Имена и форма полей точно соответствуют schema.graphqls.  При изменении
// контракта правки нужны и в этом файле, и в bff-onboarding/graph.

import { gql } from "@apollo/client";

export const QUERY_ME = gql`
  query Me {
    me {
      userId
      tenantId
      role
      email
    }
  }
`;

export const QUERY_MY_APPLICATIONS = gql`
  query MyApplications {
    myApplications {
      id
      tenantId
      state
      legalEntityType
      channel
      productCodes
      createdAt
      updatedAt
    }
  }
`;

export const QUERY_APPLICATION = gql`
  query Application($id: ID!) {
    application(id: $id) {
      id
      tenantId
      state
      legalEntityType
      channel
      productCodes
      createdAt
      updatedAt
      applicant {
        id
        fullName
        inn
        phone
        email
      }
      documents {
        id
        type
        applicationId
        filename
        state
        uploadedAt
      }
      riskAssessment {
        id
        score
        category
        recommendation
        computedAt
      }
    }
  }
`;

export const MUTATION_PREQUALIFY = gql`
  mutation Prequalify($input: PrequalifyInput!) {
    prequalify(input: $input) {
      decision
      decisionReason
      egrulStatus
      egrulFullName
      egrulCeoName
      egrulRegistrationDate
      egrulAddress
      nameMatchesEgrul
      rosfinmonPresent
      fsspProceedingsCount
      unavailableSources
      checkedAt
    }
  }
`;

export const MUTATION_SUBMIT_APPLICATION = gql`
  mutation SubmitApplication($input: SubmitApplicationInput!) {
    submitApplication(input: $input) {
      id
      state
      legalEntityType
      channel
      productCodes
      createdAt
    }
  }
`;

// ── Этап 3 — AML-сведения о деятельности ───────────────────────────────
//
// См. docs/onboarding-form-spec.md §3 и schema.graphqls типы
// ApplicationActivity / SubmitApplicationActivityInput.  Глубоко-вложенные
// поля (top-suppliers, top-buyers, operational-model, funds-source) BFF
// принимает как scalar JSON — это позволяет фронту посылать ровно ту же
// форму, которую ждёт onboarding-orchestrator.

export const QUERY_APPLICATION_ACTIVITY = gql`
  query ApplicationActivity($applicationId: ID!) {
    applicationActivity(applicationId: $applicationId) {
      id
      applicationId
      businessDescription
      businessCategory
      topSuppliers {
        name
        inn
        country
        sharePercent
        relationshipType
      }
      topBuyers {
        name
        inn
        country
        sharePercent
        relationshipType
      }
      operationalModel {
        geography
        monthlyTurnoverPlanned {
          amount
          currency
        }
        annualTurnoverPlanned {
          amount
          currency
        }
        cashSharePercent
        foreignEconomicActivity
        foreignCountries
        currencyOperations
      }
      fundsSource {
        category
        description
      }
      createdAt
      updatedAt
    }
  }
`;

export const MUTATION_SUBMIT_APPLICATION_ACTIVITY = gql`
  mutation SubmitApplicationActivity($input: SubmitApplicationActivityInput!) {
    submitApplicationActivity(input: $input) {
      id
      applicationId
      businessDescription
      businessCategory
      updatedAt
    }
  }
`;

// ── Этап 4 — ЕИО и представители ───────────────────────────────────────
//
// См. docs/onboarding-form-spec.md §4 и schema.graphqls типы
// Representative / UpsertRepresentativeInput.  ID-документ, адреса,
// authority, pdlDeclaration BFF принимает как scalar JSON.

export const QUERY_REPRESENTATIVES = gql`
  query Representatives($applicationId: ID!) {
    representatives(applicationId: $applicationId) {
      id
      applicationId
      legalEntityId
      lastName
      firstName
      middleName
      birthDate
      birthPlace
      citizenship
      inn
      snils
      idDocument {
        docType
        series
        number
        issueDate
        expiryDate
        issuedBy
        departmentCode
      }
      authority {
        position
        authorityBasis
        authorityDocNumber
        authorityDocDate
      }
      pdlDeclaration {
        isPdl
        category
        position
        relation
      }
      isPrimary
      isSignatory
      createdAt
      updatedAt
    }
  }
`;

export const MUTATION_UPSERT_REPRESENTATIVE = gql`
  mutation UpsertRepresentative($input: UpsertRepresentativeInput!) {
    upsertRepresentative(input: $input) {
      id
      applicationId
      legalEntityId
      lastName
      firstName
      middleName
      birthDate
      isPrimary
      isSignatory
      updatedAt
    }
  }
`;
