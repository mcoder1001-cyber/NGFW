import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { installFakeApi, resetSession, signIn } from '../../../test-api';
import i18n from '../../../i18n';
import { PkiActions } from './PkiActions';

afterEach(async () => {
  await resetSession();
  await i18n.changeLanguage('en');
});

describe('PKI actions', () => {
  it('readonly can export public material and cannot mutate the store', async () => {
    const fake = installFakeApi('readonly');
    await signIn();
    render(<PkiActions onChanged={vi.fn()} />);
    expect(screen.queryByRole('button', { name: 'Create CA' })).not.toBeInTheDocument();
    fake.on('GET /api/v1/actions/pki/export/gateway', {
      body: {
        name: 'gateway',
        kind: 'certificate',
        ref: 'cert/gateway',
        pem: 'PUBLIC PEM',
        fingerprint: null,
      },
    });
    fireEvent.click(screen.getByRole('button', { name: 'Export public PEM' }));
    fireEvent.change(screen.getByLabelText('Name'), { target: { value: 'gateway' } });
    fireEvent.click(screen.getByRole('button', { name: 'Submit' }));
    await screen.findByDisplayValue('PUBLIC PEM');
    expect(fake.calls.find((c) => c.path.includes('/pki/export/'))?.search).toContain(
      'kind=certificate',
    );
  });
  it('CSR wizard sends subject/SAN/key selection without returning a private key', async () => {
    const fake = installFakeApi('admin');
    await signIn();
    const changed = vi.fn();
    render(<PkiActions onChanged={changed} />);
    fake.on('POST /api/v1/actions/pki/csr', {
      body: {
        name: 'gateway',
        keyRef: 'key/gateway',
        csr: { subject: 'CN=Gateway', san: [], keySpec: { type: 'ecdsa', curve: 'p256' } },
        csrPem: 'PUBLIC CSR',
      },
    });
    fireEvent.click(screen.getByRole('button', { name: 'Create CSR' }));
    fireEvent.change(screen.getByLabelText('Name'), { target: { value: 'gateway' } });
    fireEvent.change(screen.getByLabelText('Subject'), { target: { value: 'CN=Gateway' } });
    fireEvent.change(screen.getByLabelText('Alternative names (comma separated)'), {
      target: { value: 'gw.example, 10.0.0.1' },
    });
    fireEvent.click(screen.getByRole('button', { name: 'Submit' }));
    await screen.findByDisplayValue('PUBLIC CSR');
    expect(fake.calls.find((c) => c.path.endsWith('/pki/csr'))?.body).toMatchObject({
      name: 'gateway',
      subject: 'CN=Gateway',
      san: ['gw.example', '10.0.0.1'],
      keySpec: { type: 'ecdsa', curve: 'p256' },
      replace: false,
    });
    expect(changed).toHaveBeenCalledOnce();
  });
  it('imports a public-only peer certificate without sending a private key', async () => {
    const fake = installFakeApi('admin');
    await signIn();
    render(<PkiActions onChanged={vi.fn()} />);
    fake.on('POST /api/v1/actions/pki/import', {
      body: {
        name: 'peer',
        as: 'certificate',
        certificateRef: 'cert/peer',
        keyRef: null,
        chainLength: 1,
        issued: {},
        staged: true,
      },
    });
    fireEvent.click(screen.getByRole('button', { name: 'Import' }));
    fireEvent.change(screen.getByLabelText('Name'), { target: { value: 'peer' } });
    fireEvent.change(screen.getByLabelText('Certificate PEM'), {
      target: { value: 'PUBLIC CERT' },
    });
    fireEvent.change(screen.getByLabelText('Private key PEM (optional, write only)'), {
      target: { value: 'DISCARDED_INPUT' },
    });
    fireEvent.click(
      screen.getByRole('checkbox', { name: 'Peer public certificate only (no private key)' }),
    );
    expect(
      screen.queryByLabelText('Private key PEM (optional, write only)'),
    ).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Submit' }));
    await waitFor(() =>
      expect(fake.calls.find((c) => c.path.endsWith('/pki/import'))?.body).toMatchObject({
        publicOnly: true,
        certificatePem: 'PUBLIC CERT',
      }),
    );
    expect(fake.calls.find((c) => c.path.endsWith('/pki/import'))?.body).not.toHaveProperty(
      'privateKeyPem',
    );
  });
  it('failed PEM import clears sensitive fields and hides server diagnostics', async () => {
    const fake = installFakeApi('admin');
    await signIn();
    render(<PkiActions onChanged={vi.fn()} />);
    fake.on('POST /api/v1/actions/pki/import', {
      status: 400,
      body: { detail: 'PRIVATE_DIAGNOSTIC_CANARY' },
    });
    fireEvent.click(screen.getByRole('button', { name: 'Import' }));
    fireEvent.change(screen.getByLabelText('Name'), { target: { value: 'gateway' } });
    fireEvent.change(screen.getByLabelText('Certificate PEM'), {
      target: { value: 'PUBLIC CERT' },
    });
    fireEvent.change(screen.getByLabelText('Private key PEM (optional, write only)'), {
      target: { value: 'SECRET_INPUT_CANARY' },
    });
    fireEvent.click(screen.getByRole('button', { name: 'Submit' }));
    await screen.findByText(/The action failed/);
    expect(screen.getByLabelText('Private key PEM (optional, write only)')).toHaveValue('');
    expect(screen.getByLabelText('Certificate PEM')).toHaveValue('');
    expect(document.body.textContent).not.toContain('PRIVATE_DIAGNOSTIC_CANARY');
    fireEvent.click(screen.getByRole('button', { name: 'Close' }));
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
  });
  it('Persian labels cover actions and sensitive input fields', async () => {
    await i18n.changeLanguage('fa');
    installFakeApi('admin');
    await signIn();
    render(<PkiActions onChanged={vi.fn()} />);
    expect(screen.getByRole('button', { name: 'ساخت مرجع گواهی' })).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'وارد کردن' }));
    expect(screen.getByLabelText('کلید خصوصی PEM (اختیاری، فقط ورودی)')).toBeInTheDocument();
  });
});

it('distinguishes stored material from successfully staged configuration', async () => {
  const fake = installFakeApi('admin');
  await signIn();
  render(<PkiActions onChanged={vi.fn()} />);
  fake.on('POST /api/v1/actions/pki/ca', { body: { certificatePem: 'PUBLIC CA', staged: false } });
  fireEvent.click(screen.getByRole('button', { name: 'Create CA' }));
  fireEvent.change(screen.getByLabelText('Name'), { target: { value: 'root' } });
  fireEvent.change(screen.getByLabelText('Subject'), { target: { value: 'CN=Root' } });
  fireEvent.click(screen.getByRole('button', { name: 'Submit' }));
  await screen.findByText(/Material was stored, but the candidate was not changed/);
  expect(screen.getByLabelText('Public output')).toHaveValue('PUBLIC CA');
});
