FROM phusion/baseimage:0.9.16
RUN apt-get update
RUN apt-get install haproxy

ADD ./bin/k8s-haproxy /k8s-haproxy
ADD ./haproxy.cfg /etc/haproxy/haproxy.cfg
ADD ./haproxy.cfg.gotemplate /etc/k8s-haproxy/haproxy.cfg.gotemplate
ADD ./reload-haproxy.sh /reload-haproxy.sh
ADD run /etc/service/haproxy/run

CMD /sbin/my_init
