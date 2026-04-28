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
