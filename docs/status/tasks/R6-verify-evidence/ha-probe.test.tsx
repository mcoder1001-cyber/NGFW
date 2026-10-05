import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { cleanup, render, screen, waitFor } from '@testing-library/react';
import { I18nextProvider } from 'react-i18next';
import { afterEach, expect, it, vi } from 'vitest';
import i18n from '../../../../apps/web/src/i18n';
import { StateSyncPanel } from '../../../../apps/web/src/domains/system/ha-state-sync/Panel';
const mock=vi.hoisted(()=>({fail:false,role:'admin'}));
vi.mock('../../../../apps/web/src/auth/AuthProvider',()=>({usePermissions:()=>({role:mock.role})}));
vi.mock('../../../../apps/web/src/api',()=>({api:{GET:async()=>{
 if(mock.fail) throw new Error('OWN_FIXTURE_UNAVAILABLE');
 return {response:new Response(null,{status:200}),data:{owner:'w18',kinds:[{kind:'nat44-ei',supported:true,configured:true,active:true,reason:'native'},{kind:'ipsec',supported:false,configured:true,active:false,reason:'rekey'}],lastResync:null,lastMissedCount:null,resyncCount:'0',packetCountersAvailable:false,actionsAllowed:true,observationError:'',retrievedAt:''}};
},POST:async()=>({response:new Response(null,{status:200}),data:{summary:'done'}})}}));
afterEach(()=>{cleanup();mock.fail=false;mock.role='admin'});
it('reproduces active cached ownership and enabled resync after failed refresh',async()=>{
 await i18n.changeLanguage('en');
 const client=new QueryClient({defaultOptions:{queries:{retry:false}}});
 render(<I18nextProvider i18n={i18n}><QueryClientProvider client={client}><StateSyncPanel/></QueryClientProvider></I18nextProvider>);
 await waitFor(()=>expect(screen.getByRole('button',{name:'Resync NAT44-EI'})).toBeEnabled());
 mock.fail=true; await client.invalidateQueries({queryKey:['state','ha','sync']});
 expect(await screen.findByText('Unable to read HA state')).toBeInTheDocument();
 expect(screen.getByRole('button',{name:'Resync NAT44-EI'})).toBeDisabled();
 expect(screen.queryByText('Endpoints active')).not.toBeInTheDocument();
 console.log('VERIFY: failed refresh suppresses active cache and disables Resync');
});
