FROM docker.io/searxng/searxng:latest

# Pre-seed settings before the image's own entrypoint runs
COPY settings.yml /etc/searxng/settings.yml

EXPOSE 8080