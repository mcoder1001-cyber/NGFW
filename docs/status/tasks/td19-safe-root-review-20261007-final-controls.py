"""Read-only/parser and harmless startup sentinel verification on final source."""
import importlib.util
import json
from pathlib import Path
spec=importlib.util.spec_from_file_location('fixture','scripts/tests/td19-safe-root-fixtures.py')
m=importlib.util.module_from_spec(spec); spec.loader.exec_module(m)
f=m.Fixture()
try:
    f.existing_go()
    marker=f.base/'outside-python-startup'
    imports=f.base/'imports'; imports.mkdir()
    (imports/'sitecustomize.py').write_text('from pathlib import Path\nPath('+repr(str(marker))+').write_text("startup import executed")\n')
    f.env['PYTHONPATH']=str(imports)
    f.env['PYTHONSTARTUP']=str(imports/'sitecustomize.py')
    f.env['NGFW_VPP_ARTIFACTS']=str(f.base/'not-created')
    for script,args in [('20-install-build.sh',[]),('40-install-lab.sh',[]),('00-add-repos.sh',['--dry-run'])]:
        result=f.run(script,args)
        assert result.returncode==0,result.stderr
        assert not marker.exists()
        print(json.dumps({'case':'isolated-python-startup','script':script,'returncode':result.returncode,'external_marker':marker.exists()}))
finally:
    f.close()
f=m.Fixture()
try:
    marker=f.base/'outside-os-release'
    etc=f.root/'etc'; etc.mkdir()
    (etc/'os-release').write_text('VERSION_CODENAME=$(printf unsafe > '+str(marker)+')\n')
    source=(m.ROOT/'scripts/00-add-repos.sh').read_text()
    source=source.replace('source "$ROOT/scripts/install-common.sh"','source '+json.dumps(str(m.ROOT/'scripts/install-common.sh')),1)
    source=source.replace('\ncheck_key_pins\n','\npreflight_artifacts() { :; }\ncheck_key_pins\n',1)
    script=f.base/'entry.sh';script.write_text(source)
    import subprocess
    result=subprocess.run(['bash',str(script)],env=dict(f.env,NGFW_VPP_ARTIFACTS=str(f.base)),capture_output=True,text=True)
    assert result.returncode!=0 and 'one literal VERSION_CODENAME' in result.stderr,result.stderr
    assert not marker.exists()
    assert f.calls()==[]
    print(json.dumps({'case':'nonexecutable-os-release-shell-text','returncode':result.returncode,'external_marker':marker.exists(),'effect_calls':f.calls()}))
finally:
    f.close()
