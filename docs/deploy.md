# Deploying

Each step below is a complete way to run the app. Move up only when you need what the next one adds. The app itself never changes between steps; only what sits around it does. That works because the app:

- reads all of its settings from environment variables;
- keeps no state of its own (sessions live in PostgreSQL), so any number of copies can run side by side;
- migrates the database on startup, behind a lock, so several copies starting at once are safe;
- answers `GET /healthz` (200 when it can reach the database) and logs JSON to stdout;
- finishes in-flight requests when it receives `SIGTERM`.

| Step | Where | Cost | You get | You still handle |
|---|---|---|---|---|
| 1 | This machine | free | Try it out | — |
| 2 | Your PC, through Cloudflare Tunnel | free | A public HTTPS site, no open ports | Uptime of your PC, backups |
| 3 | A server with a public IP (VPS, EC2, Lightsail) | ~$5–10/month | Always on, your own HTTPS | OS updates, backups |
| 4 | Managed AWS (ECS Fargate + RDS + load balancer) | ~$50+/month | Zero-downtime deploys, several instances, managed backups, Multi-AZ | The AWS bill and setup |

Before any deploy: `./verify.sh` passes, and you've looked at the change with `./preview.sh`.

---

## Step 1: This machine

```sh
cp .env.example .env                  # COMPOSE_FILE=docker-compose.yml:deploy/local.yml
docker compose up -d --build          # http://localhost:8080
```

## Step 2: Your PC, through Cloudflare Tunnel

The tunnel connects outwards to Cloudflare, so this works behind a home router or CGNAT, with nothing opened. Your domain must use Cloudflare DNS.

1. At https://one.dash.cloudflare.com, go to **Networks → Tunnels → Create a tunnel → Cloudflared**, name it after the app, choose **Docker**, and copy the token after `--token`.
2. Add a **published application route**, for example `myapp.example.com` → `http://app:8080`. It must be **http**: the app speaks plain HTTP inside Docker, and Cloudflare provides the HTTPS.
3. In `.env`, set `COMPOSE_FILE=docker-compose.yml:deploy/cloudflare.yml`, `DOMAIN`, `TUNNEL_TOKEN`, and a long random `POSTGRES_PASSWORD`.
4. Run `docker compose up -d --build`, then `docker compose logs -f cloudflared` and wait for `Registered tunnel connection`.

Each app gets its own tunnel and its own Compose project (named after its folder), so several apps share one PC without clashing.

Limits: the site is down while the PC is off or asleep, and Cloudflare terminates HTTPS, so it can see the traffic.

## Step 3: A server with a public IP

Any Linux VM works: a VPS from any provider, or an AWS **EC2** or **Lightsail** instance (2 GB of memory is plenty).

1. Install Docker on the server, and allow inbound ports 22, 80 and 443 (on AWS, in the instance's security group).
2. Point a DNS `A` record for your domain at the server's IP (on AWS, attach an Elastic IP first so the address doesn't change).
3. Copy the project to the server (`git clone`), then create `.env` with `COMPOSE_FILE=docker-compose.yml:deploy/caddy.yml`, `DOMAIN`, `ACME_EMAIL` and `POSTGRES_PASSWORD`.
4. Run `docker compose up -d --build`. Caddy gets a Let's Encrypt certificate on first start.

Only one stack per server can bind ports 80 and 443. To host several apps on one server, run one Caddy with a site block per app.

**Moving from step 2 to step 3:** back up the database (see README), copy `.env`, start the stack on the server, restore the backup, then switch DNS from the tunnel to the server.

**Running several copies:** `docker compose up -d --scale app=3`. Caddy spreads requests across them and skips any that fail `/healthz`. The sign-in rate limit is counted per copy.

### Deploy a published image instead of building on the server

On every push to `main`, CI (`.github/workflows/ci.yml`) runs `./verify.sh` and then publishes the image to GitHub's registry as `ghcr.io/<you>/<repo>:<commit-sha>`. Deploying that exact, tested build:

```sh
# in .env on the server
APP_IMAGE=ghcr.io/you/your-app:3f2c1ab...   # the commit to run

docker compose pull app && docker compose up -d --no-build
```

To **roll back**, set `APP_IMAGE` to the previous commit and run the same command. (The database is not rolled back, so keep each migration compatible with the previous release. See "Migrations" below.) If the repository is private, run `docker login ghcr.io` on the server first, with a token that can read packages.

## Step 4: Managed AWS

Use this when you need deploys without downtime, more than one server, or backups and failover that someone else runs. Everything the app needs maps onto a managed service:

| Here | On AWS |
|---|---|
| `docker compose build` | CI pushes the image to **ECR** (or keep using GHCR) |
| the `app` container | an **ECS service on Fargate**: 2+ tasks across availability zones, rolling deploys |
| the `db` container | **RDS for PostgreSQL**, with automated backups (Multi-AZ when uptime matters) |
| Caddy or Cloudflare | an **Application Load Balancer** with a free **ACM** certificate |
| `.env` | task definition environment, with secrets from **Secrets Manager** or **SSM Parameter Store** |
| `docker compose logs` | **CloudWatch Logs** (the JSON logs are searchable as they are) |

Settings for the ECS task:

| Variable | Value |
|---|---|
| `DATABASE_URL` | `postgres://app:<password>@<rds-endpoint>:5432/app?sslmode=require`, injected as a secret |
| `TRUST_PROXY` | `true`: the load balancer adds the client's IP as the last `X-Forwarded-For` entry |
| `PUBLIC_ORIGIN` | `https://your-domain` |
| `COOKIE_SECURE` | `true` (the default) |

- **Health check:** point the target group at `/healthz`. The container health check can be `["CMD", "/server", "healthcheck"]`.
- **Network:** tasks and RDS in private subnets. Only the load balancer is public, and RDS only accepts connections from the tasks' security group.
- **Connections:** each task opens up to 20 database connections (`internal/db`). Keep tasks × 20 under the RDS instance's limit.
- **Deploys:** register a task definition with the new image tag and update the service. ECS starts new tasks, waits for them to pass `/healthz`, then drains the old ones, which finish their requests on `SIGTERM`.

Set this up with infrastructure as code (Terraform/OpenTofu, or AWS CDK) rather than by clicking, so it can be reviewed and rebuilt. It isn't included here, because untested infrastructure code goes stale; write it when you actually make this move.

**Moving from step 3 to step 4:** create the RDS database, restore your backup into it (`psql "$DATABASE_URL" < backup.sql`), start the ECS service, check it through the load balancer's address, then point DNS at the load balancer.

---

## Migrations

Migrations run on startup and only go forward. During a rolling deploy (step 4, or step 3 with several copies), the old and new versions run against the same database for a short while. So every migration must keep the previous release working:

- Adding a table, a nullable column, or a column with a default is safe.
- Renaming or dropping a column takes two releases: first stop using it, then drop it.
- Never edit a migration that has already been deployed. Add a new one.
