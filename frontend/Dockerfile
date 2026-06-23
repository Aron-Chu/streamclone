FROM node:22-alpine AS build
WORKDIR /app
COPY frontend/package.json frontend/package-lock.json* ./frontend/
COPY packages/pulse-core ./packages/pulse-core
WORKDIR /app/frontend
# devDependencies (tailwindcss, postcss) must be present for vite build CSS.
RUN npm ci --include=dev || npm install --include=dev
COPY frontend/ ./
COPY packages/pulse-core ../packages/pulse-core
RUN npm run build

FROM nginx:1.27-alpine
COPY frontend/nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /app/frontend/dist /usr/share/nginx/html
COPY frontend/docker-entrypoint.d/40-streamclone-config.sh /docker-entrypoint.d/40-streamclone-config.sh
RUN chmod +x /docker-entrypoint.d/40-streamclone-config.sh
EXPOSE 80
