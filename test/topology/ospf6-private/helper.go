package agent

func ospf6PeerDoc(t *testing.T,slot,n int,itf string,announce bool)*structpb.Struct {
 o:=map[string]any{"routerId":fmt.Sprintf("10.%d.%d.2",slot,n),"areas":map[string]any{"0":map[string]any{}},"interfaces":map[string]any{itf:map[string]any{"area":"0","networkType":"point-to-point","helloIntervalSec":1,"deadIntervalSec":4}}}
 routing:=map[string]any{"ospf6":o}
 if announce {var statics []any;for k:=0;k<50;k++ {statics=append(statics,map[string]any{"prefix":fmt.Sprintf("fd66:%x:%x::/64",slot,64*n+k),"blackhole":true,"frr":true})};routing["static"]=statics;o["redistribute"]=map[string]any{"static":map[string]any{}}}
 out,err:=structpb.NewStruct(map[string]any{"routing":routing});if err!=nil {t.Fatal(err)};return out
}
func ospf6PrivateLive(t *testing.T,e *ospfEnv,start func()) {
 document:=func(enabled bool)*ngfwv1.DesiredState {
  ds:=e.doc(false)
  for i,name:=range []string{"host-"+e.prefix+"l0","host-"+e.prefix+"w0"} {ds.Interfaces[name].Ipv6=[]string{fmt.Sprintf("fd06:%x:%x::1/64",e.slot,i+1)}}
  if enabled {ds.Routing=&ngfwv1.RoutingConfig{Ospf6:&ngfwv1.Ospf6Config{RouterId:proto.String(fmt.Sprintf("10.%d.0.1",e.slot)),Areas:map[string]*ngfwv1.OspfArea{"0":{}},Interfaces:map[string]*ngfwv1.Ospf6Interface{}}};for _,name:=range []string{"host-"+e.prefix+"l0","host-"+e.prefix+"w0"} {ds.Routing.Ospf6.Interfaces[name]=&ngfwv1.Ospf6Interface{Area:proto.String("0"),NetworkType:proto.String("point-to-point"),HelloIntervalSec:proto.Uint32(1),DeadIntervalSec:proto.Uint32(4)}}};return ds
 }
 peer:=func(i int,announce bool){itf:=e.prefix+"l1";if i==1 {itf=e.prefix+"w1"};applyFRR(t,e.peerFRR[i],ospf6PeerDoc(t,e.slot,i+1,itf,announce))}
 e.apply("ospf6-commit",document(true));e.peers(true)
 for i,side:=range []string{"lan","wan"} {itf:=e.prefix+"l1";if i==1 {itf=e.prefix+"w1"};e.must("ip","-n","ns-"+e.prefix+"-"+side,"-6","addr","replace",fmt.Sprintf("fd06:%x:%x::2/64",e.slot,i+1),"dev",itf)}
 count:=func()(int,int){
  raw,err:=e.rootFRR.Renderer().ShowJSON(context.Background(),frr.ShowCommand("show ipv6 route ospf6 json"));if err!=nil {t.Fatal("real IPv6 FRR route observation",err)}
  var routes map[string]json.RawMessage;if err=json.Unmarshal(raw,&routes);err!=nil {t.Fatal("actual IPv6 FRR route JSON",err)}
  n:=0;for prefix:=range routes {if strings.HasPrefix(prefix,fmt.Sprintf("fd66:%x:",e.slot)) {n++}}
  ctx,cancel:=context.WithTimeout(context.Background(),30*time.Second);defer cancel();r,err:=e.c.ListRoutes(ctx,&ngfwv1.ListRoutesRequest{Family:"ipv6",Prefix:fmt.Sprintf("fd66:%x::/32",e.slot),Source:"lcp-rt-dynamic",Limit:1});if err!=nil {t.Fatal("actual IPv6 VPP FIB",err)};return n,int(r.GetTotal())
 }
 wait:=func(label string,want int,limit time.Duration){begin:=time.Now();for {rib,fib:=count();if rib==want&&fib==want {t.Logf("%s IPv6 FRR=%d VPP=%d after%s",label,rib,fib,time.Since(begin).Round(time.Millisecond));return};if time.Since(begin)>limit {t.Fatalf("%s expected%d IPv6FRR=%d IPv6VPP=%d",label,want,rib,fib)};time.Sleep(250*time.Millisecond)}}
 full:=func(limit time.Duration){begin:=time.Now();for {
  states,err:=ospf.PollNeighbors6(context.Background(),func(ctx context.Context,cmd frr.ShowCommand)(json.RawMessage,error){return e.rootFRR.Renderer().ShowJSON(ctx,cmd)})
  ok:=err==nil&&len(states)==2;for _,state:=range states {ok=ok&&state=="Full"};if ok {t.Logf("both actual OSPFv3 Full: %v",states);return};if time.Since(begin)>limit {t.Fatalf("actual OSPFv3 Full missing: states=%v err=%v",states,err)};time.Sleep(250*time.Millisecond)
 }}
 full(90*time.Second);wait("two IPv6 peers50each",100,60*time.Second)
 for _,cmd:=range [][]string{{"vppctl","show","ip6","fib",fmt.Sprintf("fd66:%x:40::/64",e.slot)},{"vppctl","show","ip6","mfib","ff02::/16"},{"ip","-6","route","show","proto","ospf"}} {out,err:=e.cmd(cmd[0],cmd[1:]...);if err!=nil {t.Fatal("IPv6 native evidence",err)};t.Logf("IPv6 evidence %v\n%s",cmd,out)}
 peer(0,false);wait("IPv6 peer withdrawal",50,10*time.Second);peer(0,true);full(30*time.Second);wait("IPv6 peer restoration",100,30*time.Second)
 e.a.Stop();names,err:=dumpNames(context.Background(),e.raw);if err!=nil {t.Fatal(err)}
 for _,name:=range []string{"host-"+e.prefix+"l0","host-"+e.prefix+"w0"} {if _,err=lcpapi.NewServiceClient(e.raw).LcpItfPairAddDelV3(context.Background(),&lcpapi.LcpItfPairAddDelV3{IsAdd:false,SwIfIndex:interface_types.InterfaceIndex(names[name])});err!=nil {t.Fatal("owned IPv6 pair loss",err)}}
 begin:=time.Now();start();full(30*time.Second);wait("actual restart lost pairs IPv6 recovery",100,30*time.Second);if time.Since(begin)>30*time.Second {t.Fatal("IPv6 restart recovery exceeded30s")};t.Logf("IPv6 restart recovery%s",time.Since(begin).Round(time.Millisecond))
 peer(0,false);wait("recreated IPv6 pair hears withdrawal",50,10*time.Second);peer(0,true);wait("recreated IPv6 pair restores100",100,30*time.Second)
 e.apply("ospf6-rollback",document(false));wait("IPv6 routing rollback zero",0,30*time.Second)
 if strings.Contains(e.runningConfig(),"router ospf6") {t.Fatal("rollback left running OSPFv3 config")}
 got,err:=e.c.Retrieve(context.Background(),&ngfwv1.RetrieveRequest{Subsystems:[]string{"routing"}});if err!=nil||got.GetDesiredState().GetRouting().GetOspf6()!=nil {t.Fatal("rollback Retrieve retained OSPFv3",err)}
 t.Log("REAL_OSPF6_PRIVATE_IPV6_LIFECYCLE=PASS")
}
