FROM node:22-alpine AS base
# pnpm is installed explicitly (pinned to package.json's packageManager version) rather than
# through corepack, which newer Node images no longer bundle.
RUN npm install -g pnpm@12.4.2 && npm cache clean --force
WORKDIR /app

FROM base AS deps
COPY package.json pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile

FROM base AS build
COPY --from=deps /app/node_modules ./node_modules
COPY . .
ENV NEXT_TELEMETRY_DISABLED=1
RUN pnpm build

FROM node:22-alpine AS runtime
ENV NODE_ENV=production NEXT_TELEMETRY_DISABLED=1 PORT=3100 HOSTNAME=0.0.0.0
WORKDIR /app
RUN addgroup -S app && adduser -S -G app app
COPY --from=build /app/.next/standalone ./
COPY --from=build /app/.next/static ./.next/static
USER app
EXPOSE 3100
CMD ["node", "server.js"]
