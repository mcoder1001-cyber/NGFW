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
import { UsersController } from './users/users.controller.js';
import { UsersService } from './users/users.service.js';
// Feature modules: `import { <slug>Feature } from './features/<slug>/index.js';` under the feature's anchor.
// wave-BC: F-det44-map-dslite-cnat
// wave-BC: F-tunnels
// wave-BC: P10
// wave-BC: F-vrrp-config-sync
// wave-BC: F-pki
// wave-BC: F-ikev2-native
// wave-BC: F-ospf
// wave-BC: F-isis-rip
// wave-BC: P14
// wave-BC: F-mpls-srmpls
// wave-BC: F-lb
// wave-BC: F-qos-flat
import { qosFlatFeature } from './features/qos-flat/index.js';
// wave-BC: F-host-stack
// wave-BC: F-snmp
// wave-BC: F-ipfix-sflow
// wave-BC: F-capture-trace
// wave-BC: F-srv6
// wave-BC: F-lisp
// wave-BC: F-bfd-redistribution
// wave-BC: F-ra-vpn
// wave-BC: F-mpls-ldp
// wave-BC: F-igmp-mfib
// wave-BC: F-dashboard-prom-alarms
// wave-BC: F-hardening-lite
// wave-BC: F-aaa
// wave-BC: F-licensing
// wave-BC: F-restconf-yang
// wave-BC: F-ha-state-sync
// wave-BC: F-ab-upgrade
// wave-BC: F-images
// wave-BC: F-backup-restore
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
// wave-A: F-nat44-ei-64-66-nptv6
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
        UsersController,
        // Feature controllers: `...<slug>Feature.controllers,` under the feature's anchor (wave-A-hotspots P1).
        // wave-BC: F-det44-map-dslite-cnat
        // wave-BC: F-tunnels
        // wave-BC: P10
        // wave-BC: F-vrrp-config-sync
        // wave-BC: F-pki
        // wave-BC: F-ikev2-native
        // wave-BC: F-ospf
        // wave-BC: F-isis-rip
        // wave-BC: P14
        // wave-BC: F-mpls-srmpls
        // wave-BC: F-lb
        // wave-BC: F-qos-flat
        ...qosFlatFeature.controllers,
        // wave-BC: F-host-stack
        // wave-BC: F-snmp
        // wave-BC: F-ipfix-sflow
        // wave-BC: F-capture-trace
        // wave-BC: F-srv6
        // wave-BC: F-lisp
        // wave-BC: F-bfd-redistribution
        // wave-BC: F-ra-vpn
        // wave-BC: F-mpls-ldp
        // wave-BC: F-igmp-mfib
        // wave-BC: F-dashboard-prom-alarms
        // wave-BC: F-hardening-lite
        // wave-BC: F-aaa
        // wave-BC: F-licensing
        // wave-BC: F-restconf-yang
        // wave-BC: F-ha-state-sync
        // wave-BC: F-ab-upgrade
        // wave-BC: F-images
        // wave-BC: F-backup-restore
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
        // wave-A: F-nat44-ei-64-66-nptv6
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
        UsersService,
        // Feature providers: `...<slug>Feature.providers,` under the feature's anchor (wave-A-hotspots P1).
        // wave-BC: F-det44-map-dslite-cnat
        // wave-BC: F-tunnels
        // wave-BC: P10
        // wave-BC: F-vrrp-config-sync
        // wave-BC: F-pki
        // wave-BC: F-ikev2-native
        // wave-BC: F-ospf
        // wave-BC: F-isis-rip
        // wave-BC: P14
        // wave-BC: F-mpls-srmpls
        // wave-BC: F-lb
        // wave-BC: F-qos-flat
        ...qosFlatFeature.providers,
        // wave-BC: F-host-stack
        // wave-BC: F-snmp
        // wave-BC: F-ipfix-sflow
        // wave-BC: F-capture-trace
        // wave-BC: F-srv6
        // wave-BC: F-lisp
        // wave-BC: F-bfd-redistribution
        // wave-BC: F-ra-vpn
        // wave-BC: F-mpls-ldp
        // wave-BC: F-igmp-mfib
        // wave-BC: F-dashboard-prom-alarms
        // wave-BC: F-hardening-lite
        // wave-BC: F-aaa
        // wave-BC: F-licensing
        // wave-BC: F-restconf-yang
        // wave-BC: F-ha-state-sync
        // wave-BC: F-ab-upgrade
        // wave-BC: F-images
        // wave-BC: F-backup-restore
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
        // wave-A: F-nat44-ei-64-66-nptv6
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
