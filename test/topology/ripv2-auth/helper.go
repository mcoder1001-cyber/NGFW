package agent

func ripPeerDoc(t *testing.T, slot,n int,itf string, announce bool)*structpb.Struct {
 r:=map[string]any{"version":2,"interfaces":map[string]any{itf:map[string]any{}}}
 routing:=map[string]any{"rip":r}
 if announce {var statics []any;for k:=0;k<50;k++ {statics=append(statics,map[string]any{"prefix":fmt.Sprintf("10.%d.%d.0/24",slot,64*n+k),"blackhole":true,"frr":true})};routing["static"]=statics;r["redistribute"]=map[string]any{"static":map[string]any{}}}
 s,err:=structpb.NewStruct(map[string]any{"routing":routing});if err!=nil {t.Fatal(err)};return s
}
func ripAuthLive(t *testing.T,e *ospfEnv,start func()) {
 secret,rotated:=os.Getenv("NGFW_IGP_PASSWORD_ONE"),os.Getenv("NGFW_IGP_PASSWORD_TWO")
 if len(secret)!=12||len(rotated)!=12||secret==rotated {t.Fatal("missing ephemeral passwords")}
 document:=func(auth bool)*ngfwv1.DesiredState {
  ds:=e.doc(false);ds.Routing=&ngfwv1.RoutingConfig{Rip:&ngfwv1.RipConfig{Version:proto.Uint32(2),Interfaces:map[string]*ngfwv1.RipInterface{}}}
  for _,name:=range []string{"host-"+e.prefix+"l0","host-"+e.prefix+"w0"} {itf:=&ngfwv1.RipInterface{};if auth {itf.Auth=&ngfwv1.OspfAuth{Type:proto.String("md5"),KeyId:proto.Uint32(7),KeyRef:proto.String("password/rip")}};ds.Routing.Rip.Interfaces[name]=itf};return ds
 }
 apply:=func(label,password string,auth bool,timeout uint32){
  ctx,cancel:=context.WithTimeout(context.Background(),90*time.Second);defer cancel()
  req:=&ngfwv1.ApplyRequest{TxnId:e.prefix+"-rip-"+label,DesiredState:document(auth),Subsystems:[]string{"interfaces","routing"},ConfirmTimeoutSec:timeout}
  if password!="" {req.SecretBundle=&ngfwv1.SecretBundle{Values:map[string][]byte{"password/rip":[]byte(password)}}}
  resp,err:=e.c.Apply(ctx,req);if err!=nil||resp.GetStatus()!=ngfwv1.ApplyStatus_APPLY_STATUS_APPLIED {t.Fatalf("sealed RIP Apply %s failed: %v status=%v",label,err,resp.GetStatus())};t.Logf("sealed RIP Apply %s APPLIED",label)
 }
 peers:=func(password string,announce bool){for i,h:=range e.peerFRR {
  itf:=e.prefix+"l1";if i==1 {itf=e.prefix+"w1"};doc:=ripPeerDoc(t,e.slot,i+1,itf,announce)
  if password!="" {doc.Fields["routing"].GetStructValue().Fields["rip"].GetStructValue().Fields["interfaces"].GetStructValue().Fields[itf].GetStructValue().Fields["auth"],_=structpb.NewValue(map[string]any{"type":"md5","keyId":7,"keyRef":"password/rip"})}
  renderer:=h.Renderer(frr.WithSecretResolver(frr.SecretResolverFunc(func(context.Context,string)(string,error){return password,nil})));files,err:=renderer.Render(context.Background(),doc);if err!=nil {t.Fatal(err)};h.AssertScoped(t,files);if err=renderer.Apply(context.Background(),files);err!=nil {t.Fatal("peer RIP Apply failed",err)}
 }}
 wait:=func(label string,want int,limit time.Duration){
  begin:=time.Now();for {rib:=int(e.state().GetRibCounts()["ipv4/default/rip"]);fib:=e.vppFRRRoutes();if rib==want&&fib==want {t.Logf("%s FRR RIP=%d VPP=%d after %s",label,rib,fib,time.Since(begin).Round(time.Millisecond));return};if time.Since(begin)>limit {t.Fatalf("%s expected%d FRR=%d VPP=%d",label,want,rib,fib)};time.Sleep(250*time.Millisecond)}
 }
 apply("enable",secret,true,0);e.peers(true);wait("unauthenticated peers rejected",0,30*time.Second)
 peers(secret,true)
 for i,r:=range []*frr.Renderer{e.rootFRR.Renderer(),e.peerFRR[0].Renderer(),e.peerFRR[1].Renderer()} {for _,command:=range []string{"show ip rip status","show running-config","show ip route rip"} {out,err:=r.Show(context.Background(),frr.ShowCommand(command));t.Logf("RIP diagnostic instance%d %s err=%v\n%s",i,command,err,out)}}
 for _,args:=range [][]string{{"vppctl","show","ip","mfib","224.0.0.0/24"},{"timeout","45","tcpdump","-nn","-c","4","-i",e.prefix+"-l0","udp","port","520"},{"ip","netns","exec","ns-"+e.prefix+"-lan","timeout","45","tcpdump","-nn","-c","4","-i",e.prefix+"l1","udp","port","520"}} {out,err:=e.cmd(args[0],args[1:]...);t.Logf("RIP diagnostic %v err=%v\n%s",args,err,out)}
 wait("matching MD5 peers100",100,90*time.Second)
 apply("rotate",rotated,true,0);wait("old password peers expire",0,210*time.Second)
 peers(rotated,true);wait("new password peers100",100,90*time.Second)
 e.a.Stop();start();wait("production restart sealed RIP100",100,90*time.Second)
 apply("confirm",secret,true,240);wait("candidate rejects rotated peers",0,210*time.Second)
 peers(secret,true);wait("candidate old generation100",100,90*time.Second)
 wait("actual timeout baseline rejects old peers",0,450*time.Second)
 peers(rotated,true);wait("historical rotated baseline100",100,90*time.Second)
 apply("remove-auth","",false,0);wait("removed auth rejects MD5 peers",0,210*time.Second)
 peers("",true);wait("both unauthenticated100",100,90*time.Second)
 e.apply("rip-rollback",e.doc(false));wait("routing rollback zero",0,30*time.Second)
 t.Log("REAL_RIPV2_MD5_SAME_REF_LIFECYCLE=PASS")
}
