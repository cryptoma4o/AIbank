{{- define "risk-engine.fullname" -}}
{{- printf "%s-%s" .Release.Name "risk-engine" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "risk-engine.labels" -}}
helm.sh/chart: {{ .Chart.Name }}-{{ .Chart.Version }}
app.kubernetes.io/name: risk-engine
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{- define "risk-engine.selectorLabels" -}}
app.kubernetes.io/name: risk-engine
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}
