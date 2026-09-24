import { type DynamicModule, Inject, Module, type OnApplicationShutdown } from '@nestjs/common';
import { APP_FILTER, APP_GUARD, APP_INTERCEPTOR } from '@nestjs/core';
import { ActionsController } from './actions/actions.controller.js';
import { AgentClient } from './agent/agent.client.js';
import { AuditController } from './audit/audit.controller.js';
import { AuditInterceptor } from './audit/audit.interceptor.js';
import { AuditService } from './audit/audit.service.js';
import { SystemEventsService } from './audit/system-events.service.js';
import { AuthController } from './auth/auth.controller.js';
import { AuthGuard } from './auth/auth.guard.js';
import { AuthService } from './auth/auth.service.js';
import { TokensService } from './auth/tokens.service.js';
import { CommitService } from './commit/commit.service.js';
import { ValidationService } from './commit/validation.service.js';
import { ProblemFilter } from './common/problem.js';
import { ConfigController } from './config/config.controller.js';
import { ENV, loadEnv, type Env } from './config.js';
import { CONFIG_REPO, DatastoreService } from './datastore/datastore.service.js';
import { PgConfigRepo } from './datastore/pg-repo.js';
import { createDb, DB, type DbHandle } from './db/db.js';
import { HealthController } from './health/health.controller.js';
import { Bus } from './infra/bus.js';
import { createValkey, VALKEY, type Valkey } from './infra/valkey.js';
import { SecretsController } from './secrets/secrets.controller.js';
import { SecretsService } from './secrets/secrets.service.js';
import { StateController } from './state/state.controller.js';
import { RelayService } from './telemetry/relay.service.js';
// Feature modules: `import { <slug>Feature } from './features/<slug>/index.js';` under the feature's anchor.
// wave-A: F-bonding
// wave-A: F-bridge-l2
// wave-A: F-loopback-bvi-gso-lldp-span
// wave-A: F-vrf-static-ecmp
// wave-A: F-neighbors-ra
// wave-A: F-rpf-adl-pbr
// wave-A: F-object-model
// wave-A: F-acl
// wave-A: F-host-acl-nftables
// wave-A: F-nat44-ed-sessions
import { nat44EdSessionsFeature } from './features/nat44-ed-sessions/index.js';
// wave-A: F-nat44-ei-64-66-nptv6
import { nat44Ei6466Nptv6Feature } from './features/nat44-ei-64-66-nptv6/index.js';
// wave-A: P11
// wave-A: F-wireguard
// wave-A: P12
// wave-A: F-kea-dhcp-relay
// wave-A: F-unbound-chrony-syslog

const DB_HANDLE = Symbol('VRX_DB_HANDLE');

/** Closes the pool and the Valkey client when the application shuts down. */
class Resources implements OnApplicationShutdown {
  constructor(
    @Inject(DB_HANDLE) private readonly db: DbHandle,
    @Inject(VALKEY) private readonly kv: Valkey,
  ) {}

  async onApplicationShutdown(): Promise<void> {
    await this.db.close().catch(() => undefined);
    this.kv.disconnect();
  }
}

/**
 * The vrx-api application. `AppModule.forRoot(env)` wires everything from one parsed environment; nothing connects at
 * construction (database, Valkey and agent are lazy), so the OpenAPI generator and the route-guard test build the
 * complete app offline. Global: AuthGuard on every route, AuditInterceptor on every mutation, problem+json filter.
 */
@Module({})
export class AppModule {
  static forRoot(env: Env = loadEnv()): DynamicModule {
    return {
      module: AppModule,
      controllers: [
        HealthController,
        AuthController,
        ConfigController,
        StateController,
        ActionsController,
        SecretsController,
        AuditController,
        // Feature controllers: `...<slug>Feature.controllers,` under the feature's anchor (wave-A-hotspots P1).
        // wave-A: F-bonding
        // wave-A: F-bridge-l2
        // wave-A: F-loopback-bvi-gso-lldp-span
        // wave-A: F-vrf-static-ecmp
        // wave-A: F-neighbors-ra
        // wave-A: F-rpf-adl-pbr
        // wave-A: F-object-model
        // wave-A: F-acl
        // wave-A: F-host-acl-nftables
        // wave-A: F-nat44-ed-sessions
        ...nat44EdSessionsFeature.controllers,
        // wave-A: F-nat44-ei-64-66-nptv6
        ...nat44Ei6466Nptv6Feature.controllers,
        // wave-A: P11
        // wave-A: F-wireguard
        // wave-A: P12
        // wave-A: F-kea-dhcp-relay
        // wave-A: F-unbound-chrony-syslog
      ],
      providers: [
        { provide: ENV, useValue: env },
        { provide: DB_HANDLE, useFactory: () => createDb(env) },
        { provide: DB, useFactory: (h: DbHandle) => h.db, inject: [DB_HANDLE] },
        { provide: VALKEY, useFactory: () => createValkey(env) },
        { provide: CONFIG_REPO, useClass: PgConfigRepo },
        Resources,
        Bus,
        AgentClient,
        AuditService,
        SystemEventsService,
        TokensService,
        AuthService,
        DatastoreService,
        ValidationService,
        CommitService,
        SecretsService,
        RelayService,
        // Feature providers: `...<slug>Feature.providers,` under the feature's anchor (wave-A-hotspots P1).
        // wave-A: F-bonding
        // wave-A: F-bridge-l2
        // wave-A: F-loopback-bvi-gso-lldp-span
        // wave-A: F-vrf-static-ecmp
        // wave-A: F-neighbors-ra
        // wave-A: F-rpf-adl-pbr
        // wave-A: F-object-model
        // wave-A: F-acl
        // wave-A: F-host-acl-nftables
        // wave-A: F-nat44-ed-sessions
        ...nat44EdSessionsFeature.providers,
        // wave-A: F-nat44-ei-64-66-nptv6
        ...nat44Ei6466Nptv6Feature.providers,
        // wave-A: P11
        // wave-A: F-wireguard
        // wave-A: P12
        // wave-A: F-kea-dhcp-relay
        // wave-A: F-unbound-chrony-syslog
        { provide: APP_GUARD, useClass: AuthGuard },
        { provide: APP_INTERCEPTOR, useClass: AuditInterceptor },
        { provide: APP_FILTER, useClass: ProblemFilter },
      ],
    };
  }
}
