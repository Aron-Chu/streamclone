FROM node:22-alpine AS build
WORKDIR /app
COPY package.json package-lock.json* ./
RUN npm ci || npm install
COPY . .
RUN npm run build

FROM nginx:1.27-alpine
COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /app/dist /usr/share/nginx/html
COPY docker-entrypoint.d/40-streamclone-config.sh /docker-entrypoint.d/40-streamclone-config.sh
RUN chmod +x /docker-entrypoint.d/40-streamclone-config.sh
EXPOSE 80
