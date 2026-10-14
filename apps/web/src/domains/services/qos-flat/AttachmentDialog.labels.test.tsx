import { InterfaceLabelsProvider, NgfwThemeProvider } from '@ngfw/ui-kit';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import '../../../i18n';
import { AttachmentDialog } from './AttachmentDialog';
import type { AttachmentCfg } from './model';

const NAME = 'wan';
const LABELS = new Map([[NAME, 'Ethernet1']]);

describe('QoS interface and object labels', () => {
  it.each(['policer', 'shaper'] as const)(
    'keeps %s and marking-map object names when they match a physical port',
    async (kind) => {
      const onSubmit = vi.fn();
      const cfg: AttachmentCfg = {
        policer: { input: NAME, ...(kind === 'policer' ? { output: NAME } : {}) },
        ...(kind === 'shaper' ? { shaper: NAME } : {}),
        mark: { map: NAME, output: 'ip' },
      };
      render(
        <NgfwThemeProvider mode="light" lang="en" dir="ltr">
          <InterfaceLabelsProvider labels={LABELS}>
            <AttachmentDialog
              target={{ name: NAME, cfg, editing: true }}
              existing={[NAME]}
              interfaceOptions={[NAME]}
              policers={[NAME]}
              shapers={[NAME]}
              maps={[NAME]}
              error={null}
              pending={false}
              onCancel={vi.fn()}
              onSubmit={onSubmit}
            />
          </InterfaceLabelsProvider>
        </NgfwThemeProvider>,
      );
      expect(screen.getByLabelText('Interface')).toHaveValue('Ethernet1');
      expect(screen.getByRole('combobox', { name: 'Ingress policer' })).toHaveTextContent(NAME);
      expect(
        screen.getByRole('combobox', {
          name: kind === 'policer' ? 'Egress policer' : 'Rate limit',
        }),
      ).toHaveTextContent(NAME);
      expect(screen.getByRole('combobox', { name: 'Marking map' })).toHaveTextContent(NAME);
      fireEvent.click(screen.getByRole('button', { name: 'Save to candidate' }));
      await waitFor(() => expect(onSubmit).toHaveBeenCalledWith(NAME, cfg));
    },
  );
});
