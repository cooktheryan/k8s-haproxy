FROM phusion/baseimage:0.9.16

RUN sed -i 's/^# \(.*-backports\s\)/\1/g' /etc/apt/sources.list
RUN apt-get update
RUN apt-get install -y haproxy=1.5.3-1~ubuntu14.04.1
RUN sed -i 's/^ENABLED=.*/ENABLED=1/' /etc/default/haproxy

ADD ./bin/k8s-haproxy /k8s-haproxy
ADD ./haproxy.cfg /etc/haproxy/haproxy.cfg
ADD ./haproxy.cfg.gotemplate /etc/k8s-haproxy/haproxy.cfg.gotemplate
ADD ./reload-haproxy.sh /reload-haproxy.sh
ADD run /etc/service/haproxy/run

CMD /sbin/my_init
