ARG KAFKA_IMAGE=apache/kafka:4.4.0
FROM ${KAFKA_IMAGE}

USER root

ADD https://github.com/prometheus/jmx_exporter/releases/download/1.5.0/jmx_prometheus_javaagent-1.5.0.jar /opt/kafka/libs/jmx_prometheus_javaagent.jar
COPY jmx-exporter.yml /opt/kafka/config/jmx-exporter.yml

RUN chown appuser:appuser /opt/kafka/libs/jmx_prometheus_javaagent.jar /opt/kafka/config/jmx-exporter.yml

USER appuser
