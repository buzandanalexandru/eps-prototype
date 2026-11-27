{{/*
eps.service renders the Deployment, Service and (optionally) Ingress of one Go binary.
Call from a sub-chart: {{ include "eps.service" (dict "name" "core" "ctx" .) }}
*/}}
{{- define "eps.labels" -}}
app.kubernetes.io/name: {{ .name }}
app.kubernetes.io/instance: {{ .ctx.Release.Name }}
app.kubernetes.io/part-of: eps
eps.dev/env: {{ .ctx.Values.global.env | quote }}
{{- end -}}

{{- define "eps.fullname" -}}
{{ printf "%s-%s" .ctx.Release.Name .name | trunc 63 | trimSuffix "-" }}
{{- end -}}

{{- define "eps.image" -}}
{{ printf "%s/eps-%s:%s" .ctx.Values.global.image.registry .name (.ctx.Values.global.image.tag | toString) }}
{{- end -}}

{{- define "eps.env" -}}
- name: PORT
  value: "8080"
- name: DATABASE_URL
  valueFrom:
    secretKeyRef:
      name: {{ .ctx.Values.global.databaseSecret }}
      key: DATABASE_URL
      optional: true
- name: REDIS_URL
  value: {{ printf "redis://%s-redis:6379/0" .ctx.Release.Name | quote }}
- name: CORS_ALLOWED_ORIGINS
  value: {{ .ctx.Values.global.corsAllowedOrigins | quote }}
- name: EPS_ENV
  value: {{ .ctx.Values.global.env | quote }}
{{- end -}}

{{- define "eps.podSecurity" -}}
securityContext:
  runAsNonRoot: true
  runAsUser: 65532
  runAsGroup: 65532
  seccompProfile:
    type: RuntimeDefault
{{- end -}}

{{- define "eps.containerSecurity" -}}
securityContext:
  allowPrivilegeEscalation: false
  readOnlyRootFilesystem: true
  capabilities:
    drop: ["ALL"]
{{- end -}}

{{- define "eps.service" -}}
{{- $v := .ctx.Values -}}
{{- if $v.enabled }}
apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ include "eps.fullname" . }}
  labels:
    {{- include "eps.labels" . | nindent 4 }}
spec:
  replicas: {{ $v.replicas | default 1 }}
  revisionHistoryLimit: 3
  selector:
    matchLabels:
      app.kubernetes.io/name: {{ .name }}
      app.kubernetes.io/instance: {{ .ctx.Release.Name }}
  template:
    metadata:
      labels:
        {{- include "eps.labels" . | nindent 8 }}
    spec:
      automountServiceAccountToken: false
      {{- include "eps.podSecurity" . | nindent 6 }}
      containers:
        - name: {{ .name }}
          image: {{ include "eps.image" . }}
          imagePullPolicy: {{ .ctx.Values.global.image.pullPolicy }}
          ports:
            - name: http
              containerPort: 8080
          env:
            {{- include "eps.env" . | nindent 12 }}
          livenessProbe:
            httpGet: { path: /healthz, port: http }
            periodSeconds: 10
            failureThreshold: 3
          readinessProbe:
            httpGet: { path: /readyz, port: http }
            periodSeconds: 5
            failureThreshold: 3
          startupProbe:
            httpGet: { path: /healthz, port: http }
            periodSeconds: 2
            failureThreshold: 30
          resources:
            {{- toYaml $v.resources | nindent 12 }}
          {{- include "eps.containerSecurity" . | nindent 10 }}
---
apiVersion: v1
kind: Service
metadata:
  name: {{ include "eps.fullname" . }}
  labels:
    {{- include "eps.labels" . | nindent 4 }}
spec:
  selector:
    app.kubernetes.io/name: {{ .name }}
    app.kubernetes.io/instance: {{ .ctx.Release.Name }}
  ports:
    - name: http
      port: 80
      targetPort: http
{{- if and $v.ingress $v.ingress.enabled }}
{{- $host := printf "%s.%s.%s" $v.ingress.host .ctx.Values.global.env .ctx.Values.global.baseDomain }}
---
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: {{ include "eps.fullname" . }}
  labels:
    {{- include "eps.labels" . | nindent 4 }}
  annotations:
    cert-manager.io/cluster-issuer: {{ .ctx.Values.global.ingress.clusterIssuer }}
spec:
  ingressClassName: {{ .ctx.Values.global.ingress.className }}
  tls:
    - hosts: [{{ $host | quote }}]
      secretName: {{ printf "%s-%s-tls" .name .ctx.Values.global.env | trunc 63 | trimSuffix "-" }}
  rules:
    - host: {{ $host | quote }}
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: {{ include "eps.fullname" . }}
                port:
                  name: http
{{- end }}
{{- end }}
{{- end -}}
