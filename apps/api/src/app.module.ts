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
import { LoginBannerController } from './state/login-banner.controller.js';
import { StateController } from './state/state.controller.js';
import { RelayService } from './telemetry/relay.service.js';
import { UsersController } from './users/users.controller.js';
import { UsersService } from './users/users.service.js';
// Feature modules: `import { <slug>Feature } from './features/<slug>/index.js';` under the feature's anchor.
// wave-BC: F-det44-map-dslite-cnat
import { det44MapDsliteCnatFeature } from './features/det44-map-dslite-cnat/index.js';
import { nat46Feature } from './features/nat46/index.js'; // F-nat46 (unanchored)
// wave-BC: F-tunnels
// wave-BC: P10
// wave-BC: F-vrrp-config-sync
// wave-BC: F-pki
import { pkiFeature } from './features/pki/index.js';
// wave-BC: F-ikev2-native
// wave-BC: F-ospf
// wave-BC: F-isis-rip
// wave-BC: P14
// wave-BC: F-mpls-srmpls
import { mplsSrmplsFeature } from './features/mpls-srmpls/index.js';
// wave-BC: F-lb
import { lbFeature } from './features/lb/index.js';
import { ruleExpiryFeature } from './features/rule-expiry/index.js'; // F-rule-expiry (unanchored)
import { globalBlockingFeature } from './features/global-blocking/index.js'; // F-global-blocking (unanchored)
import { pppoeFeature } from './features/pppoe/index.js'; // F-pppoe-client (unanchored)
import { dashboardPromAlarmsFeature } from './features/dashboard-prom-alarms/index.js'; // wave-BC: F-dashboard-prom-alarms
import { multiwanFeature } from './features/multiwan/index.js'; // F-multiwan (unanchored)
import { aaaFeature } from './features/aaa/index.js'; // wave-BC: F-aaa
import { autoBlockFeature } from './features/auto-block/index.js'; // F-bruteforce-block (unanchored)
import { restconfYangFeature } from './features/restconf-yang/index.js'; // wave-BC: F-restconf-yang
// wave-BC: F-qos-flat
import { qosFlatFeature } from './features/qos-flat/index.js';
// wave-BC: F-host-stack
import { hostStackFeature } from './features/host-stack/index.js';
// wave-BC: F-snmp
import { snmpFeature } from './features/snmp/index.js';
import { dataplaneFeature } from './features/dataplane/index.js'; // F-dataplane-ui (unanchored)
import { mgmtTlsFeature } from './features/mgmt-tls/index.js'; // F-management-ui (unanchored)
// wave-BC: F-ipfix-sflow
import { ipfixSflowFeature } from './features/ipfix-sflow/index.js';
// wave-BC: F-capture-trace
import { captureTraceFeature } from './features/capture-trace/index.js';
// wave-BC: F-srv6
import { srv6Feature } from './features/srv6/index.js';
// wave-BC: F-lisp
import { lispFeature } from './features/lisp/index.js';
// wave-BC: F-bfd-redistribution
// wave-BC: F-ra-vpn
// wave-BC: F-mpls-ldp
import { mplsLdpFeature } from './features/mpls-ldp/index.js';
// wave-BC: F-igmp-mfib
import { igmpMfibFeature } from './features/igmp-mfib/index.js';
// wave-BC: F-dashboard-prom-alarms
// web: WEB-dashboard
import { hostMetricsFeature } from './features/host-metrics/index.js';
// wave-BC: F-hardening-lite
// wave-BC: F-aaa
// wave-BC: F-licensing
import { licensingFeature } from './features/licensing/index.js';
// wave-BC: F-restconf-yang
// wave-BC: F-ha-state-sync
// wave-BC: F-ab-upgrade
// wave-BC: F-images
// wave-BC: F-backup-restore
// wave-A: F-bonding
import { bondingFeature } from './features/bonding/index.js';
// wave-A: F-bridge-l2
import { bridgeL2Feature } from './features/bridge-l2/index.js';
// wave-A: F-loopback-bvi-gso-lldp-span
import { loopbackBviGsoLldpSpanFeature } from './features/loopback-bvi-gso-lldp-span/index.js';
// wave-A: F-vrf-static-ecmp
import { vrfStaticEcmpFeature } from './features/vrf-static-ecmp/index.js';
// wave-A: F-neighbors-ra
import { neighborsRaFeature } from './features/neighbors-ra/index.js';
// wave-A: F-rpf-adl-pbr
import { rpfAdlPbrFeature } from './features/rpf-adl-pbr/index.js';
// wave-A: F-object-model
import { objectModelFeature } from './features/object-model/index.js';
// wave-A: F-acl
import { aclFeature } from './features/acl/index.js';
// wave-A: F-host-acl-nftables
import { hostAclNftablesFeature } from './features/host-acl-nftables/index.js';
// wave-A: F-nat44-ed-sessions
import { nat44EdSessionsFeature } from './features/nat44-ed-sessions/index.js';
// wave-A: F-nat44-ei-64-66-nptv6
import { nat44Ei6466Nptv6Feature } from './features/nat44-ei-64-66-nptv6/index.js';
// wave-A: P11
// wave-A: F-wireguard
import { wireguardFeature } from './features/wireguard/index.js';
// wave-A: P12
import { bgpFeature } from './features/bgp/index.js';
// wave-A: F-kea-dhcp-relay
import { keaDhcpRelayFeature } from './features/kea-dhcp-relay/index.js';
// wave-A: F-unbound-chrony-syslog
import { unboundChronySyslogFeature } from './features/unbound-chrony-syslog/index.js';

import { notificationsFeature } from './features/notifications/index.js';

const DB_HANDLE = Symbol('NGFW_DB_HANDLE');

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
 * The ngfw-api application. `AppModule.forRoot(env)` wires everything from one parsed environment; nothing connects at
 * construction (database, Valkey and agent are lazy), so the OpenAPI generator and the route-guard test build the
 * complete app offline. Global: AuthGuard on every route, AuditInterceptor on every mutation, problem+json filter.
 */
@Module({})
export class AppModule {
  static forRoot(env: Env = loadEnv()): DynamicModule {
    return {
      module: AppModule,
      controllers: [
        ...notificationsFeature.controllers,
        HealthController,
        AuthController,
        ConfigController,
        StateController,
        LoginBannerController,
        ActionsController,
        SecretsController,
        AuditController,
        UsersController,
        // Feature controllers: `...<slug>Feature.controllers,` under the feature's anchor (wave-A-hotspots P1).
        // wave-BC: F-det44-map-dslite-cnat
        ...det44MapDsliteCnatFeature.controllers,
        ...nat46Feature.controllers, // F-nat46 (unanchored)
        // wave-BC: F-tunnels
        // wave-BC: P10
        // wave-BC: F-vrrp-config-sync
        // wave-BC: F-pki
        ...pkiFeature.controllers,
        // wave-BC: F-ikev2-native
        // wave-BC: F-ospf
        // wave-BC: F-isis-rip
        // wave-BC: P14
        // wave-BC: F-mpls-srmpls
        ...mplsSrmplsFeature.controllers,
        // wave-BC: F-lb
        ...lbFeature.controllers,
        // wave-BC: F-qos-flat
        ...qosFlatFeature.controllers,
        // wave-BC: F-host-stack
        ...hostStackFeature.controllers,
        // wave-BC: F-snmp
        ...snmpFeature.controllers,
        ...dataplaneFeature.controllers, // F-dataplane-ui (unanchored)
        ...mgmtTlsFeature.controllers, // F-management-ui (unanchored)
        // wave-BC: F-ipfix-sflow
        ...ipfixSflowFeature.controllers,
        // wave-BC: F-capture-trace
        ...captureTraceFeature.controllers,
        // wave-BC: F-srv6
        ...srv6Feature.controllers,
        // wave-BC: F-lisp
        ...lispFeature.controllers,
        // wave-BC: F-bfd-redistribution
        // wave-BC: F-ra-vpn
        // wave-BC: F-mpls-ldp
        ...mplsLdpFeature.controllers,
        // wave-BC: F-igmp-mfib
        ...igmpMfibFeature.controllers,
        // wave-BC: F-dashboard-prom-alarms
        // web: WEB-dashboard
        ...hostMetricsFeature.controllers,
        // wave-BC: F-hardening-lite
        // wave-BC: F-aaa
        // wave-BC: F-licensing
        ...licensingFeature.controllers,
        // wave-BC: F-restconf-yang
        // wave-BC: F-ha-state-sync
        // wave-BC: F-ab-upgrade
        // wave-BC: F-images
        // wave-BC: F-backup-restore
        // wave-A: F-bonding
        ...bondingFeature.controllers,
        // wave-A: F-bridge-l2
        ...bridgeL2Feature.controllers,
        // wave-A: F-loopback-bvi-gso-lldp-span
        ...loopbackBviGsoLldpSpanFeature.controllers,
        // wave-A: F-vrf-static-ecmp
        ...vrfStaticEcmpFeature.controllers,
        // wave-A: F-neighbors-ra
        ...neighborsRaFeature.controllers,
        // wave-A: F-rpf-adl-pbr
        ...rpfAdlPbrFeature.controllers,
        // wave-A: F-object-model
        ...objectModelFeature.controllers,
        // wave-A: F-acl
        ...aclFeature.controllers,
        ...globalBlockingFeature.controllers, // F-global-blocking (unanchored)
        ...pppoeFeature.controllers, // F-pppoe-client (unanchored)
        ...dashboardPromAlarmsFeature.controllers, // wave-BC: F-dashboard-prom-alarms
        ...multiwanFeature.controllers, // F-multiwan (unanchored)
        ...aaaFeature.controllers, // wave-BC: F-aaa
        ...autoBlockFeature.controllers, // F-bruteforce-block (unanchored)
        ...restconfYangFeature.controllers, // wave-BC: F-restconf-yang
        // wave-A: F-host-acl-nftables
        ...hostAclNftablesFeature.controllers,
        // wave-A: F-nat44-ed-sessions
        ...nat44EdSessionsFeature.controllers,
        // wave-A: F-nat44-ei-64-66-nptv6
        ...nat44Ei6466Nptv6Feature.controllers,
        // wave-A: P11
        // wave-A: F-wireguard
        ...wireguardFeature.controllers,
        // wave-A: P12
        ...bgpFeature.controllers,
        // wave-A: F-kea-dhcp-relay
        ...keaDhcpRelayFeature.controllers,
        // wave-A: F-unbound-chrony-syslog
        ...unboundChronySyslogFeature.controllers,
      ],
      providers: [
        { provide: ENV, useValue: env },
        { provide: DB_HANDLE, useFactory: () => createDb(env) },
        { provide: DB, useFactory: (h: DbHandle) => h.db, inject: [DB_HANDLE] },
        { provide: VALKEY, useFactory: () => createValkey(env) },
        { provide: CONFIG_REPO, useClass: PgConfigRepo },
        ...notificationsFeature.providers,
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
        ...det44MapDsliteCnatFeature.providers,
        ...nat46Feature.providers, // F-nat46 (unanchored)
        // wave-BC: F-tunnels
        // wave-BC: P10
        // wave-BC: F-vrrp-config-sync
        // wave-BC: F-pki
        ...pkiFeature.providers,
        // wave-BC: F-ikev2-native
        // wave-BC: F-ospf
        // wave-BC: F-isis-rip
        // wave-BC: P14
        // wave-BC: F-mpls-srmpls
        ...mplsSrmplsFeature.providers,
        // wave-BC: F-lb
        ...lbFeature.providers,
        ...ruleExpiryFeature.providers, // F-rule-expiry (unanchored)
        ...globalBlockingFeature.providers, // F-global-blocking (unanchored)
        ...dashboardPromAlarmsFeature.providers, // wave-BC: F-dashboard-prom-alarms
        ...aaaFeature.providers, // wave-BC: F-aaa
        ...autoBlockFeature.providers, // F-bruteforce-block (unanchored)
        ...restconfYangFeature.providers, // wave-BC: F-restconf-yang
        // wave-BC: F-qos-flat
        ...qosFlatFeature.providers,
        // wave-BC: F-host-stack
        // wave-BC: F-snmp
        ...snmpFeature.providers,
        // wave-BC: F-ipfix-sflow
        ...ipfixSflowFeature.providers,
        // wave-BC: F-capture-trace
        // wave-BC: F-srv6
        ...srv6Feature.providers,
        // wave-BC: F-lisp
        ...lispFeature.providers,
        // wave-BC: F-bfd-redistribution
        // wave-BC: F-ra-vpn
        // wave-BC: F-mpls-ldp
        ...mplsLdpFeature.providers,
        // wave-BC: F-igmp-mfib
        ...igmpMfibFeature.providers,
        // wave-BC: F-dashboard-prom-alarms
        // web: WEB-dashboard
        ...hostMetricsFeature.providers,
        // wave-BC: F-hardening-lite
        // wave-BC: F-aaa
        // wave-BC: F-licensing
        ...licensingFeature.providers,
        ...mgmtTlsFeature.providers, // F-management-ui (unanchored)
        // wave-BC: F-restconf-yang
        // wave-BC: F-ha-state-sync
        // wave-BC: F-ab-upgrade
        // wave-BC: F-images
        // wave-BC: F-backup-restore
        // wave-A: F-bonding
        ...bondingFeature.providers,
        // wave-A: F-bridge-l2
        ...bridgeL2Feature.providers,
        // wave-A: F-loopback-bvi-gso-lldp-span
        ...loopbackBviGsoLldpSpanFeature.providers,
        // wave-A: F-vrf-static-ecmp
        ...vrfStaticEcmpFeature.providers,
        // wave-A: F-neighbors-ra
        ...neighborsRaFeature.providers,
        // wave-A: F-rpf-adl-pbr
        ...rpfAdlPbrFeature.providers,
        // wave-A: F-object-model
        ...objectModelFeature.providers,
        // wave-A: F-acl
        ...aclFeature.providers,
        // wave-A: F-host-acl-nftables
        ...hostAclNftablesFeature.providers,
        // wave-A: F-nat44-ed-sessions
        ...nat44EdSessionsFeature.providers,
        // wave-A: F-nat44-ei-64-66-nptv6
        ...nat44Ei6466Nptv6Feature.providers,
        // wave-A: P11
        // wave-A: F-wireguard
        ...wireguardFeature.providers,
        // wave-A: P12
        ...bgpFeature.providers,
        // wave-A: F-kea-dhcp-relay
        ...keaDhcpRelayFeature.providers,
        // wave-A: F-unbound-chrony-syslog
        ...unboundChronySyslogFeature.providers,
        { provide: APP_GUARD, useClass: AuthGuard },
        { provide: APP_INTERCEPTOR, useClass: AuditInterceptor },
        { provide: APP_FILTER, useClass: ProblemFilter },
      ],
    };
  }
}
