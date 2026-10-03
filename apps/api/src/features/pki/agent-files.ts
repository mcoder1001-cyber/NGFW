import { credentials, Metadata, type ServiceError } from '@grpc/grpc-js';
import { Inject, Injectable, type OnModuleDestroy } from '@nestjs/common';
import { DataplaneClient, type PkiFileStateResponse } from '@ngfw/proto';
import { ENV, type Env } from '../../config.js';

/**
 * The agent's PkiFileState RPC (F-pki): which PKI files the agent materialised for strongSwan, by fingerprint. Its own
 * lazily created channel to the agent socket — `apps/api/src/agent/agent.client.ts` is outside F-pki's envelope (the
 * one-line `pkiFileState()` there is in docs/status/tasks/F-pki-questions.md Q2). Read-only, short deadline; any gRPC
 * failure becomes `{error}` so the state page still answers.
 */
@Injectable()
export class PkiAgentFiles implements OnModuleDestroy {
  private client: DataplaneClient | undefined;

  constructor(@Inject(ENV) private readonly env: Env) {}

  private get c(): DataplaneClient {
    this.client ??= new DataplaneClient(
      `unix:${this.env.VRX_AGENT_SOCKET}`,
      credentials.createInsecure(),
      {
        'grpc.max_reconnect_backoff_ms': 5000,
        'grpc.initial_reconnect_backoff_ms': 200,
      },
    );
    return this.client;
  }

  state(timeoutMs = 3000): Promise<PkiFileStateResponse | { error: string }> {
    return new Promise((resolve) => {
      this.c.pkiFileState(
        { owner: this.env.VRX_AGENT_OWNER },
        new Metadata(),
        { deadline: new Date(Date.now() + timeoutMs) },
        (err: ServiceError | null, res: PkiFileStateResponse) =>
          resolve(err ? { error: `agent: ${err.details || err.message}` } : res),
      );
    });
  }

  onModuleDestroy(): void {
    this.client?.close();
    this.client = undefined;
  }
}
