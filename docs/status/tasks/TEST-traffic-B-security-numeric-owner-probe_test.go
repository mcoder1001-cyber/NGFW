package keadhcprelay
import("os";"os/exec";"strings";"testing";"net/http";"net/http/httptest";"sync/atomic")
func TestR2ChangedOwnerRefusedBeforeMutation(t *testing.T){
 if os.Getenv("NGFW_R2_NUMERIC_OWNER_CHILD")=="1"{
  var locks atomic.Int32
  server:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){w.Header().Set("Content-Type","application/json");response:=`{"interfaces":{}}`;if r.URL.Path=="/api/v1/config/lock"{response=`{"locked":true,"ownerId":1}`;if locks.Add(1)>1{response=`{"locked":true,"ownerId":2}`}};if r.Method=="POST"{t.Log("MUTATION_SENT");response=`{"status":"applied","revision":{"id":1},"warnings":[],"notApplied":[]}`};_,_=w.Write([]byte(response))}));defer server.Close()
  a:=&api{t:t,base:server.URL};a.commit("kea-base");t.Fatal("changed owner accepted")
 }
 cmd:=exec.Command(os.Args[0],"-test.run=^TestR2ChangedOwnerRefusedBeforeMutation$","-test.v");cmd.Env=append(os.Environ(),"NGFW_R2_NUMERIC_OWNER_CHILD=1");out,e:=cmd.CombinedOutput()
 if e==nil||!strings.Contains(string(out),"candidate ownership changed")||strings.Contains(string(out),"MUTATION_SENT"){t.Fatalf("foreign owner refusal not proven: exit=%v output=%s",e,out)}
}
