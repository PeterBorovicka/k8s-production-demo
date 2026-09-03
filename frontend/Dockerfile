FROM nginxinc/nginx-unprivileged:1.31-alpine
COPY --chown=101:101 nginx.conf /etc/nginx/nginx.conf
COPY --chown=101:101 index.html app.js styles.css /usr/share/nginx/html/
EXPOSE 8080
USER 101:101

