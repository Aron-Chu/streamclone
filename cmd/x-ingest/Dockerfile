FROM oven/bun:1.2-alpine
WORKDIR /app
COPY package.json ./
RUN bun install --production
COPY server.js ./
ENV PORT=8098
EXPOSE 8098
CMD ["bun", "server.js"]
