{{- define "onboarding-orchestrator.fullname" -}}
{{- printf "%s-%s" .Release.Name "onboarding-orchestrator" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "onboarding-orchestrator.labels" -}}
helm.sh/chart: {{ .Chart.Name }}-{{ .Chart.Version }}
app.kubernetes.io/name: onboarding-orchestrator
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{- define "onboarding-orchestrator.selectorLabels" -}}
app.kubernetes.io/name: onboarding-orchestrator
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}
