import { NgfwWsClient } from '@ngfw/ui-kit/ws';
import { App } from '../App';
import { Session } from '../auth/session';
import { installPreviewTransport, previewResponse } from './transport';

installPreviewTransport();
const previewSession = new Session(previewResponse, () => Date.now(), { channel: null });
class OfflineStream extends NgfwWsClient {
  override connect(): void { /* No live telemetry or device connection in this preview. */ }
}
const stream = new OfflineStream({ url: 'ws://offline-preview.invalid' });

const PREVIEW_LABEL = 'UI PREVIEW · Sample data · No device connection · Changes and device actions disabled';

const PREVIEW_CSS = '.ngfw-common-shell { block-size: calc(100dvh - 34px); }';

export function PreviewApp() {
  return <>
    <div role="status" style={{ padding: '8px 16px', background: '#fff3cd', color: '#403000', font: '14px Arial', borderBottom: '1px solid #d6b64a' }}>
      {PREVIEW_LABEL}
    </div>
    <style>{PREVIEW_CSS}</style>
    <App session={previewSession} wsClient={stream} />
  </>;
}
