package main
import("fmt";"os";"path/filepath";"syscall")
func protected(root *os.Root) bool {i,e:=root.Stat(".");if e!=nil{return false};s,ok:=i.Sys().(*syscall.Stat_t);return i.IsDir()&&i.Mode().Perm()==0700&&ok&&int(s.Uid)==os.Geteuid()}
func main(){
 d,e:=os.MkdirTemp("","ngfw-r2-evidence-");if e!=nil{panic(e)};defer os.RemoveAll(d)
 outside,e:=os.MkdirTemp("","ngfw-r2-outside-");if e!=nil{panic(e)};defer os.RemoveAll(outside)
 runtime:=filepath.Join(d,"traffic-b-private-123");if e=os.Mkdir(runtime,0700);e!=nil{panic(e)}
 scratch,e:=os.OpenRoot(d);if e!=nil{panic(e)};defer scratch.Close()
 r,e:=scratch.OpenRoot("traffic-b-private-123");if e!=nil{panic(e)};defer r.Close();if !protected(r){panic("protected owned runtime refused")}
 if e=r.Mkdir("evidence",0700);e!=nil{panic(e)};ev,e:=r.OpenRoot("evidence");if e!=nil{panic(e)};defer ev.Close();if !protected(ev){panic("evidence refused")}
 f,e:=ev.OpenFile("ipsec-rest.private.log",os.O_WRONLY|os.O_CREATE|os.O_EXCL,0600);if e!=nil{panic(e)};f.Close();i,e:=ev.Stat("ipsec-rest.private.log");if e!=nil||i.Mode().Perm()!=0600{panic("log permissions")};fmt.Println("owned0700 runtime/evidence and0600 exclusive log: PASS")
 if e=os.Symlink(outside,filepath.Join(d,"escape"));e!=nil{panic(e)};if x,e:=scratch.OpenRoot("escape");e==nil{x.Close();panic("outside root symlink accepted")};fmt.Println("outside-root directory symlink refusal: PASS")
 if e=os.Symlink(filepath.Join(outside,"target"),filepath.Join(runtime,"evidence","symlink.log"));e!=nil{panic(e)};if x,e:=ev.OpenFile("symlink.log",os.O_WRONLY|os.O_CREATE|os.O_EXCL,0600);e==nil{x.Close();panic("symlink log accepted")};if _,e=os.Stat(filepath.Join(outside,"target"));!os.IsNotExist(e){panic("outside write")};fmt.Println("symlink log refusal without outside write: PASS")
 os.Chmod(runtime,0755);if protected(r){panic("unprotected runtime accepted")};os.Chmod(runtime,0700);fmt.Println("wrong directory mode refusal: PASS")
 os.Chown(runtime,65534,65534);if protected(r){panic("foreign owner accepted")};os.Chown(runtime,os.Getuid(),os.Getgid());fmt.Println("foreign directory UID refusal: PASS")
}
