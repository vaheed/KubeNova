{{- define "operator.name" -}}
{{- printf "kubenova-operator" -}}
{{- end }}

{{- define "operator.labels" -}}
app.kubernetes.io/name: {{ include "operator.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/managed-by: Helm
{{- end }}
