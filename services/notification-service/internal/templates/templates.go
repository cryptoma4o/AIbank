// Package templates - registry of notification templates.
//
// Uses text/template; templates have Subject (email/push title) and Body
// (email/SMS/push body).
//
// Texts are in Russian by default (white-label RU platform); per-tenant
// override planned via configs/tenants/<id>/branding/content.yaml
// (see docs/tenant-configuration.md).
package templates

import (
	"bytes"
	"errors"
	"fmt"
	"sort"
	tt "text/template"
)

// ErrUnknownTemplate - template_id not registered.
var ErrUnknownTemplate = errors.New("templates: unknown template")

// Rendered - render result.
type Rendered struct {
	Subject string
	Body    string
}

type def struct {
	subject *tt.Template
	body    *tt.Template
}

// Registry - pool of known templates.
type Registry struct {
	tpls map[string]*def
}

// NewRegistry creates registry with built-in templates.
func NewRegistry() (*Registry, error) {
	r := &Registry{tpls: make(map[string]*def)}
	for id, raw := range builtin {
		if err := r.register(id, raw.Subject, raw.Body); err != nil {
			return nil, fmt.Errorf("register %s: %w", id, err)
		}
	}
	return r, nil
}

// Render - resolves (id, vars) -> Rendered.
//
// Uses Option("missingkey=error"); a missing var causes a render error,
// not an empty string.
func (r *Registry) Render(id string, vars map[string]string) (*Rendered, error) {
	d, ok := r.tpls[id]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnknownTemplate, id)
	}
	subject, err := runTpl(d.subject, vars)
	if err != nil {
		return nil, fmt.Errorf("subject: %w", err)
	}
	body, err := runTpl(d.body, vars)
	if err != nil {
		return nil, fmt.Errorf("body: %w", err)
	}
	return &Rendered{Subject: subject, Body: body}, nil
}

// IDs - sorted list of registered template ids.
func (r *Registry) IDs() []string {
	out := make([]string, 0, len(r.tpls))
	for k := range r.tpls {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func (r *Registry) register(id, subject, body string) error {
	st, err := tt.New(id + ":subject").Option("missingkey=error").Parse(subject)
	if err != nil {
		return err
	}
	bt, err := tt.New(id + ":body").Option("missingkey=error").Parse(body)
	if err != nil {
		return err
	}
	r.tpls[id] = &def{subject: st, body: bt}
	return nil
}

func runTpl(t *tt.Template, vars map[string]string) (string, error) {
	if vars == nil {
		vars = map[string]string{}
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, vars); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// builtin - platform built-in templates. Texts in Russian (white-label RU).
//
// Variable contracts:
//   - applicant_welcome:    {{.full_name}}, {{.bank_name}}
//   - application_approved: {{.full_name}}, {{.bank_name}}, {{.account_number}}
//   - application_declined: {{.full_name}}, {{.bank_name}}, {{.reason}}
//   - document_request:     {{.full_name}}, {{.bank_name}}, {{.document_type}}
//   - otp:                  {{.code}}, {{.ttl_minutes}}
type builtinTpl struct{ Subject, Body string }

var builtin = map[string]builtinTpl{
	"applicant_welcome": {
		Subject: "Здравствуйте, {{.full_name}}! Заявка принята",
		Body: "Здравствуйте, {{.full_name}}!\n\n" +
			"Ваша заявка на открытие расчётного счёта в {{.bank_name}} принята в обработку.\n" +
			"Мы свяжемся с вами в течение 1 рабочего дня.\n\n" +
			"С уважением,\n{{.bank_name}}",
	},
	"application_approved": {
		Subject: "{{.bank_name}}: счёт открыт",
		Body: "Здравствуйте, {{.full_name}}!\n\n" +
			"Ваш расчётный счёт в {{.bank_name}} успешно открыт.\n" +
			"Номер счёта: {{.account_number}}\n\n" +
			"Реквизиты для контрагентов уже доступны в личном кабинете.\n\n" +
			"С уважением,\n{{.bank_name}}",
	},
	"application_declined": {
		Subject: "{{.bank_name}}: решение по заявке",
		Body: "Здравствуйте, {{.full_name}}!\n\n" +
			"К сожалению, мы не можем открыть счёт по вашей заявке.\n" +
			"Причина: {{.reason}}\n\n" +
			"При необходимости вы можете уточнить детали у вашего менеджера.\n\n" +
			"С уважением,\n{{.bank_name}}",
	},
	"document_request": {
		Subject: "{{.bank_name}}: требуются документы",
		Body: "Здравствуйте, {{.full_name}}!\n\n" +
			"Для продолжения обработки заявки нам требуется дополнительный документ:\n" +
			"{{.document_type}}\n\n" +
			"Загрузите файл в личном кабинете.\n\n" +
			"С уважением,\n{{.bank_name}}",
	},
	"otp": {
		Subject: "Код подтверждения",
		Body:    "Ваш код подтверждения: {{.code}}. Действует {{.ttl_minutes}} минут. Никому его не сообщайте.",
	},
}
