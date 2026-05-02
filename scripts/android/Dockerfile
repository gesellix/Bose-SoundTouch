FROM python:3.11-slim

ARG FRIDA_VERSION=17.9.1
ARG FRIDA_TOOLS_VERSION=14.8.1

RUN apt-get update && \
    apt-get install -y android-tools-adb xz-utils curl && \
    rm -rf /var/lib/apt/lists/*

RUN pip install frida==${FRIDA_VERSION} frida-tools==${FRIDA_TOOLS_VERSION} objection

RUN curl -L "https://github.com/frida/frida/releases/download/${FRIDA_VERSION}/frida-server-${FRIDA_VERSION}-android-arm64.xz" \
    -o /tmp/frida-server.xz && \
    xz -d /tmp/frida-server.xz && \
    mv /tmp/frida-server /usr/local/bin/frida-server-android && \
    chmod +x /usr/local/bin/frida-server-android

ARG FRIDA_SCRIPTS_REPO=https://github.com/httptoolkit/frida-interception-and-unpinning
ARG FRIDA_SCRIPTS_REF=main
RUN mkdir -p /usr/local/share/frida-scripts && \
    curl -L "${FRIDA_SCRIPTS_REPO}/archive/refs/heads/${FRIDA_SCRIPTS_REF}.tar.gz" \
    -o /tmp/frida-scripts.tar.gz && \
    tar -xzf /tmp/frida-scripts.tar.gz --strip-components=1 \
    -C /usr/local/share/frida-scripts && \
    rm /tmp/frida-scripts.tar.gz
