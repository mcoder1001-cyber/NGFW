import { QueryClient } from '@tanstack/react-query';
import { cleanup, render, screen, within } from '@testing-library/react';
import { createMemoryRouter } from 'react-router';
import { afterEach, expect, it } from 'vitest';
import { App } from '../../../../apps/web/src/App';
import i18n from '../../../../apps/web/src/i18n';
import { installFakeApi, resetSession, signIn } from '../../../../apps/web/src/test-api';
import { BfdRedistributionPage, RedistributionPage } from '../../../../apps/web/src/domains/routing/bfd-redistribution/BfdRedistributionPage';
afterEach(async()=> { cleanup(); await resetSession(); localStorage.clear(); await i18n.changeLanguage('en'); });
async function mount(element: React.ReactNode) {
 await signIn();
 render(<App router={createMemoryRouter([{path:'/',element}])} streamUrl='ws://127.0.0.1:1/api/v1/stream' queryClient={new QueryClient({defaultOptions:{queries:{retry:false}}})}/>);
}
it('reproduces untranslated observed state in Persian twice separately',async()=>{
 const fake=installFakeApi('admin');
 fake.on('GET /api/v1/config/candidate/routing',()=>({body:{}}));
 fake.on('GET /api/v1/config/routing',()=>({body:{}}));
 fake.on('GET /api/v1/state/routing/bfd/sessions',()=>({body:{agentError:null,retrievedAt:null,sessions:[{engine:'vpp',interface:'w14',localAddress:'192.0.2.1',peerAddress:'192.0.2.2',state:'down',desiredMinTxUs:300000,requiredMinRxUs:300000,detectMultiplier:3,lastFlap:null,multihop:false}]}}));
 await i18n.changeLanguage('fa'); await mount(<BfdRedistributionPage/>);
 expect(await screen.findByText(i18n.t('bfd-redistribution:states.down'))).toBeInTheDocument(); expect(screen.queryByText('down')).not.toBeInTheDocument(); console.log('VERIFY: Persian observed Down translated');
});
it('verifies displayed production VRF context',async()=>{
 const fake=installFakeApi('admin'); fake.on('GET /api/v1/state/routing/redistribution',()=>({body:{agentError:null,retrievedAt:null,edges:[{source:'static',target:'ospf',vrf:'blue',routeMap:'BLUE_FILTER',routeCount:'11',readOnly:false}]}}));
 await mount(<RedistributionPage/>); expect(await screen.findByText('BLUE_FILTER (11) · VRF: blue')).toBeInTheDocument();
});
