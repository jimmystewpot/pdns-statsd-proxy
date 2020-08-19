from ubuntu:focal

RUN apt update && \
    apt -y install bash curl gnupg
    

COPY config/pdns.list /etc/apt/sources.list.d/pdns.list
COPY config/pdns-apt /etc/apt/preferences.d/pdns

RUN curl https://repo.powerdns.com/FD380FBB-pub.asc | apt-key add - && \
    apt update && \
    apt -y install pdns-recursor
