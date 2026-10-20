FROM node:24-alpine AS deps
WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci --no-audit --no-fund

# Режим разработки для docker compose (Vite dev server)
FROM deps AS dev
COPY . .
EXPOSE 5173
CMD ["npm", "run", "dev", "--", "--host", "0.0.0.0"]

# Прод: статическая сборка, раздаётся reverse proxy (Caddy, §18.6)
FROM deps AS build
COPY . .
RUN npm run build

FROM caddy:2-alpine AS prod
COPY --from=build /app/dist /srv
COPY Caddyfile /etc/caddy/Caddyfile
