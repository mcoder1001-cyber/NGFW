import {createApp} from '/usr/lib/ngfw/api/dist/app.js';
import {StateController} from '/usr/lib/ngfw/api/dist/state/state.controller.js';
import {DatastoreService} from '/usr/lib/ngfw/api/dist/datastore/datastore.service.js';
if(process.getuid()!==0)throw Error('root SSH only');
process.setgid('ngfw');process.setuid('ngfw');
const app=await createApp({docs:false,logger:['error']});
try {
 const s=await app.get(StateController).interfaces();
 const r=await app.get(DatastoreService).getRunning();
 const data=s.items.filter(x=>x.physical?.owner==='dataplane');
 if(s.observationErrors.length||data.some(x=>x.awaitingDataplane!==false||!x.state||!x.hostInventory?.boundToDpdk))throw Error('interface observation not ready');
 const management=s.items.filter(x=>x.hostInventory?.isManagement);
 if(management.some(x=>x.hostInventory.boundToDpdk))throw Error('management handed to DPDK');
 console.log(JSON.stringify({revision:r.revision?.id,dataplaneStatus:s.dataplaneStatus,hostInventoryStatus:s.hostInventoryStatus,observationErrors:s.observationErrors,dataPorts:data.length,allDataPresent:true,management:management.map(x=>({name:x.name,pci:x.hostInventory.pci,driver:x.hostInventory.driver,boundToDpdk:x.hostInventory.boundToDpdk}))}));
}finally{await app.close();}
