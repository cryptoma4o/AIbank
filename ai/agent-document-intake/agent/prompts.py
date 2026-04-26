from models.schemas import DocumentType

SYSTEM_PROMPT = (
    "Ты — система извлечения данных из российских юридических документов. "
    "Извлеки структурированные поля из текста документа. "
    "Отвечай ТОЛЬКО валидным JSON без пояснений."
)

FIELD_PROMPTS: dict[DocumentType, str] = {
    DocumentType.PASSPORT: """Извлеки из текста паспорта РФ следующие поля (JSON):
{"surname": "", "name": "", "patronymic": "", "birth_date": "YYYY-MM-DD", "series": "", "number": "", "issued_by": "", "issued_date": "YYYY-MM-DD", "department_code": ""}""",

    DocumentType.EGRUL: """Извлеки из выписки ЕГРЮЛ следующие поля (JSON):
{"full_name": "", "short_name": "", "inn": "", "ogrn": "", "kpp": "", "okved_main": "", "legal_address": "", "ceo_name": "", "registration_date": "YYYY-MM-DD", "status": ""}""",

    DocumentType.CHARTER: """Извлеки из устава организации следующие поля (JSON):
{"company_name": "", "authorized_capital_kopecks": 0, "founders": [], "director_title": "", "activity_types": []}""",

    DocumentType.INN_CERTIFICATE: """Извлеки из свидетельства ИНН следующие поля (JSON):
{"inn": "", "full_name": "", "registration_date": "YYYY-MM-DD"}""",

    DocumentType.OGRN_CERTIFICATE: """Извлеки из свидетельства ОГРН следующие поля (JSON):
{"ogrn": "", "full_name": "", "registration_date": "YYYY-MM-DD", "registering_authority": ""}""",

    DocumentType.BANK_STATEMENT: """Извлеки из банковской выписки следующие поля (JSON):
{"account_number": "", "bank_name": "", "bik": "", "period_start": "YYYY-MM-DD", "period_end": "YYYY-MM-DD", "opening_balance_kopecks": 0, "closing_balance_kopecks": 0}""",
}
