package agent

func igpAuthLive(t *testing.T, e *ospfEnv, full *ngfwv1.DesiredState, start func()) {
 t.Helper()
 secret := os.Getenv("NGFW_IGP_PASSWORD_ONE")
 rotated := os.Getenv("NGFW_IGP_PASSWORD_TWO")
 if len(secret)!=12 || len(rotated)!=12 || secret==rotated {t.Fatal("missing independent ephemeral test credentials")}
 authDoc := func(enabled bool)*ngfwv1.DesiredState {
  ds:=proto.Clone(full).(*ngfwv1.DesiredState)
  for _,itf:=range ds.Routing.Ospf.Interfaces {if enabled {itf.Auth=&ngfwv1.OspfAuth{Type:proto.String("md5"),KeyId:proto.Uint32(7),KeyRef:proto.String("password/ospf")}} else {itf.Auth=nil}}
  return ds
 }
 apply:=func(label string, ds *ngfwv1.DesiredState, password string){
  ctx,cancel:=context.WithTimeout(context.Background(),90*time.Second);defer cancel()
  request:=&ngfwv1.ApplyRequest{TxnId:e.prefix+"-auth-"+label,DesiredState:ds,Subsystems:[]string{"interfaces","routing"}}
  if label=="confirm-revert" {request.ConfirmTimeoutSec=30}
  if password!="" {request.SecretBundle=&ngfwv1.SecretBundle{Values:map[string][]byte{ds.Routing.Ospf.Interfaces["host-"+e.prefix+"l0"].Auth.GetKeyRef():[]byte(password)}}}
  response,err:=e.c.Apply(ctx,request)
  if err!=nil || response.GetStatus()!=ngfwv1.ApplyStatus_APPLY_STATUS_APPLIED {t.Fatalf("real sealed auth apply %s failed: %v status=%v",label,err,response.GetStatus())}
  t.Logf("real sealed auth Apply %s APPLIED",label)
 }
 peers:=func(password string,announce bool){
  for i,h:=range e.peerFRR {
   itf:=e.prefix+"l1";if i==1 {itf=e.prefix+"w1"}
   document:=ospfPeerDoc(t,e.slot,i+1,itf,announce)
   if password!="" {document.Fields["routing"].GetStructValue().Fields["ospf"].GetStructValue().Fields["interfaces"].GetStructValue().Fields[itf].GetStructValue().Fields["auth"],_=structpb.NewValue(map[string]any{"type":"md5","keyId":7,"keyRef":"password/ospf"})}
   renderer:=h.Renderer(frr.WithSecretResolver(frr.SecretResolverFunc(func(context.Context,string)(string,error){return password,nil})))
   files,err:=renderer.Render(context.Background(),document);if err!=nil {t.Fatal(err)}
   h.AssertScoped(t,files);if err=renderer.Apply(context.Background(),files);err!=nil {t.Fatal("real peer auth apply failed",err)}
  }
 }
 apply("enable",authDoc(true),secret)
 e.waitOSPF("MD5 refuses unauthenticated peers",0,15*time.Second)
 peers(secret,true);e.waitFull(45*time.Second);e.waitOSPF("matching MD5 learned100 routes",100,30*time.Second)
 rotatedDoc:=authDoc(true)
 if os.Getenv("NGFW_IGP_DIAGNOSTIC_NEW_REF")=="1" {for _,itf:=range rotatedDoc.Routing.Ospf.Interfaces {itf.Auth.KeyRef=proto.String("password/ospf-rotated")};t.Log("DIAGNOSTIC_NEW_REF_ONLY: same-reference rotation remains failed/unaccepted")}
 apply("rotate",rotatedDoc,rotated)
 e.waitOSPF("old passwords withdraw100 routes",0,15*time.Second)
 peers(rotated,true);e.waitFull(45*time.Second);e.waitOSPF("rotated matching credentials restore100",100,30*time.Second)
 e.a.Stop();start();e.waitFull(45*time.Second);e.waitOSPF("sealed MD5 reload on actual agent restart",100,30*time.Second)
 apply("confirm-revert",authDoc(true),secret)
 e.waitOSPF("unconfirmed old generation rejects rotated peers",0,15*time.Second)
 peers(secret,true);e.waitFull(20*time.Second);e.waitOSPF("unconfirmed candidate old generation learns100",100,15*time.Second)
 e.waitOSPF("confirm timeout historical rotated baseline rejects candidate peers",0,45*time.Second)
 peers(rotated,true);e.waitFull(45*time.Second);e.waitOSPF("confirm revert restores historical sealed baseline100",100,30*time.Second)
 peers(rotated,false);e.waitOSPF("authenticated peer route withdrawal",0,15*time.Second)
 peers(rotated,true);e.waitFull(45*time.Second);e.waitOSPF("authenticated peer route restore",100,30*time.Second)
 apply("remove-auth",authDoc(false),"")
 e.waitOSPF("auth removal refuses still-MD5 peers",0,15*time.Second)
 peers("",true);e.waitFull(45*time.Second);e.waitOSPF("both sides remove auth restore100",100,30*time.Second)
 apply("rollback",e.doc(false),"");e.waitOSPF("routing rollback removes learned routes",0,15*time.Second)
 if os.Getenv("NGFW_IGP_DIAGNOSTIC_NEW_REF")=="1" {t.Log("DIAGNOSTIC_OSPF_MD5_NEW_REF_LIFECYCLE=PASS")} else {t.Log("REAL_OSPF_MD5_SEALED_ROTATION_REMOVAL_RESTART_WITHDRAW_ROLLBACK=PASS")}
}
