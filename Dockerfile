FROM haproxy:1.5.10

RUN mkdir /var/lib/haproxy
RUN mkdir /etc/haproxy
RUN mv /usr/local/etc/haproxy/errors/ /etc/haproxy/

ADD ./bin/k8s-haproxy /k8s-haproxy
ADD ./haproxy.cfg /etc/haproxy/haproxy.cfg
ADD ./haproxy.cfg.gotemplate /etc/k8s-haproxy/haproxy.cfg.gotemplate
ADD ./reload-haproxy.sh /reload-haproxy.sh

CMD ["/k8s-haproxy", "--master=http://pilot00.qa.porch.com:8080"]
