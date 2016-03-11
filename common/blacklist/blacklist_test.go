package blacklist

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func sameslice(s1 []string, s2 []string) bool {
	if len(s1) != len(s2) {
		return false
	}
	for idx, elem := range s1 {
		if elem != s2[idx] {
			return false
		}
	}
	return true
}

func makeyaml(path string, remove bool) error {
	template := `
build.remove-blacklisted-files: %t
build.file-blacklist:
    - dir1/*
    - dir2/dir3/*
    - dir2/other.txt
    - dir2/*/baz.py
    - "[!a-z].txt"
    - toplevelfile.txt
    - nonexistent.txt
`
	contents := fmt.Sprintf(template, remove)
	return ioutil.WriteFile(path, []byte(contents), 0777)
}

func newfile(root, name string) error {
	path := filepath.Join(root, name)
	// we use trailing slash to signify a directory
	if strings.HasSuffix(name, "/") {
		if e := os.MkdirAll(path, 0777); e != nil {
			return e
		}
		return nil
	}
	if filepath.Dir(path) != root {
		if e := os.MkdirAll(filepath.Dir(path), 0777); e != nil {
			return e
		}
	}
	return ioutil.WriteFile(path, []byte("test"), 0777)
}

func newfiles(root string, names ...string) error {
	for _, n := range names {
		if e := newfile(root, n); e != nil {
			return e
		}
	}
	return nil
}

func TestParseYaml(t *testing.T) {
	dirname, err := ioutil.TempDir("", "parseyaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dirname)
	yamlfile := filepath.Join(dirname, "foo.yaml")
	if err = makeyaml(yamlfile, true); err != nil {
		t.Fatal(err)
	}
	config, err := parseYaml(yamlfile)
	if err != nil {
		t.Fatal(err)
	}
	if !config.RemoveBlacklistFiles {
		t.Error("Config incorrectly read RemoveBlacklistFiles as false")
	}

	expected := []string{"dir1/*", "dir2/dir3/*", "dir2/other.txt", "dir2/*/baz.py", "[!a-z].txt", "toplevelfile.txt", "nonexistent.txt"}
	if !sameslice(config.FileBlacklist, expected) {
		t.Errorf("Config incorrectly parsed blacklisted files. Actual: %v, Expected: %v", config.FileBlacklist, expected)
	}
}

func removeBlacklistFilesHelper(t *testing.T, tempDirName string, remove bool) {
	dirname, err := ioutil.TempDir("", tempDirName)
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dirname)
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	os.Chdir(dirname)
	defer os.Chdir(cwd)
	yamlfile := filepath.Join(dirname, "foo.yaml")
	if err = makeyaml(yamlfile, remove); err != nil {
		t.Fatal(err)
	}

	dontMatchBlacklist := []string{"dir1/", "dir2/", "dir2/dir3/", "dir2/foo.txt", "foo/toplevelfile.txt", "a.txt"}
	matchBlacklist := []string{"dir1/foo.txt", "dir1/other/", "dir1/other/bar.txt", "dir2/dir3/baz.yaml", "dir2/other.txt", "dir2/foo/bar/baz.py", "0.txt", "toplevelfile.txt"}

	var shouldExist []string
	var shouldntExist []string
	if remove {
		shouldExist = dontMatchBlacklist
		shouldntExist = matchBlacklist
	} else {
		// if yaml file says not to remove blacklisted files everything should still exist
		shouldExist = append(dontMatchBlacklist, matchBlacklist...)
		shouldntExist = []string{}
	}

	if err = newfiles(".", append(shouldExist, shouldntExist...)...); err != nil {
		t.Fatal(err)
	}

	err = RemoveBlacklistedFiles(".", "foo.yaml")
	if err != nil {
		t.Fatal(err)
	}

	for _, file := range shouldExist {
		if _, err := os.Stat(file); err != nil {
			if os.IsNotExist(err) {
				t.Errorf("File %s shouldn't have been removed but was", file)
			} else {
				t.Errorf("Error checking existence of %s: %s", file, err)
			}
		}
	}

	for _, file := range shouldntExist {
		if _, err := os.Stat(file); err != nil && !os.IsNotExist(err) {
			t.Errorf("Error checking non-existence of %s: %s", file, err)
		} else if err == nil {
			t.Errorf("File %s should have been removed but wasn't", file)
		}
	}
}

func TestRemoveBlacklistFilesTrue(t *testing.T) {
	removeBlacklistFilesHelper(t, "removeblacklistfilestrue", true)
}

func TestRemoveBlacklistFilesFalse(t *testing.T) {
	removeBlacklistFilesHelper(t, "removeblacklistfilesfalse", false)
}

func TestBlacklistNoYamlFile(t *testing.T) {
	dirname, err := ioutil.TempDir("", "blacklistnoyamlfile")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dirname)
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	os.Chdir(dirname)
	defer os.Chdir(cwd)
	err = RemoveBlacklistedFiles(".", "bar.yaml")
	if err != nil {
		t.Errorf("Encountered error when yaml file didn't exist: %s", err)
	}
}

func BenchmarkMatch(b *testing.B) {
	const matches = 4
	files := []string{
		"rlfiltr/bootstrap/foo.txt",
		"go/src/fastcar/search/test.json",
		"ixvtn/pinsot/cabernet.xml",
		"dpkg/install_me.deb",
		"meatserver/meatserver/internal/dirty/olive.py",
	}
	matcher := newMatcher(bigPatternList)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		matchcount := 0
		for _, fname := range files {
			if m, e := matcher.Match(fname); e != nil {
				panic(e)
			} else if m {
				matchcount++
			}
		}
		if matchcount != matches {
			b.Fatalf("Expected %v matches, got %v", matches, matchcount)
		}
	}
}

// Blacklist that is remarkably similar to a large one in use in the wild.
var bigPatternList = []string{
	".art_lib/*",
	"datalytics/*",
	"build_tools/*",
	"crime/*",
	"ci/iso/*",
	"ci/maps-*",
	"codesearch/*",
	"canardcenter/*",
	"configs/banddad/*",
	"configs/ci/*",
	"configs/config/global_abc.yaml",
	"configs/graphene-api-forte4/*",
	"configs/cribe-config/*",
	"configs/kaka/cfg_prod.yaml",
	"configs/kaka/config/*",
	"configs/kaka/supervisor.d/*",
	"configs/monitoring/*",
	"configs/register/abc_config_master.txt",
	"configs/register/abc_config_proxies.txt",
	"configs/register/abc_config_saves.txt",
	"configs/cribe/*",
	"configs/forte4_alerts/*",
	"cpp/blockvat/*",
	"cpp/fastcar/vacuut/*",
	"dpkg/*",
	"ixvtn.yaml",
	"ixvtn/allocation/*",
	"ixvtn/analytics/*",
	"ixvtn/analyticswebserver/*",
	"ixvtn/apx/*",
	"ixvtn/artifactory/*",
	"ixvtn/backups/*",
	"ixvtn/binbox/*",
	"ixvtn/blockvat/*",
	"ixvtn/gruemail/*",
	"ixvtn/volt/*",
	"ixvtn/BUILD",
	"ixvtn/capacity-dashboard/*",
	"ixvtn/cpnts/*",
	"ixvtn/celery/*",
	"ixvtn/ci/*",
	"ixvtn/commandcenter/*",
	"ixvtn/conman/*",
	"ixvtn/container/*",
	"ixvtn/contbun/*",
	"ixvtn/crashmash_service/*",
	"ixvtn/crons/*",
	"ixvtn/dashboards/*",
	"ixvtn/datastorks/*",
	"ixvtn/db/*",
	"ixvtn/dctools/*",
	"ixvtn/debshop/*",
	"ixvtn/desktop_notifier/*",
	"ixvtn/dns/*",
	"ixvtn/doptrack/*",
	"ixvtn/drraw/*",
	"ixvtn/dsh/*",
	"ixvtn/ecadmin/*",
	"ixvtn/mallstore/*",
	"ixvtn/email/*",
	"ixvtn/eventbot/*",
	"ixvtn/chexlog/*",
	"ixvtn/exits/*",
	"ixvtn/fio/*",
	"ixvtn/flanket/*",
	"ixvtn/flo/*",
	"ixvtn/ganglala/*",
	"ixvtn/code-actual/*",
	"ixvtn/git/*",
	"ixvtn/trouper/*",
	"ixvtn/gish/*",
	"ixvtn/hadroop/*",
	"ixvtn/snappypears/*",
	"ixvtn/haprolly/*",
	"ixvtn/hardware/*",
	"ixvtn/hegwig/*",
	"ixvtn/hermet/*",
	"ixvtn/hg/*",
	"ixvtn/hype/*",
	"ixvtn/hwmaint/*",
	"ixvtn/hyades/*",
	"ixvtn/install/*",
	"ixvtn/interviews/*",
	"ixvtn/irc/*",
	"ixvtn/mira/*",
	"ixvtn/kaka/*",
	"ixvtn/lifecycle/*",
	"ixvtn/mdb/*",
	"ixvtn/ramcached/*",
	"ixvtn/verot/*",
	"ixvtn/mibs/*",
	"ixvtn/misc/*",
	"ixvtn/mist/*",
	"ixvtn/mobilebot/*",
	"ixvtn/monit/*",
	"ixvtn/monitoring/*",
	"ixvtn/pysqlproxy/*",
	"ixvtn/nabios3/*",
	"ixvtn/net/*",
	"ixvtn/meninx/*",
	"ixvtn/rotserver/*",
	"ixvtn/nsot/*",
	"ixvtn/oncall/*",
	"ixvtn/opslog/*",
	"ixvtn/ops_server/*",
	"ixvtn/paperduly/*",
	"ixvtn/payments/*",
	"ixvtn/photosnot/*",
	"ixvtn/kingdom/*",
	"ixvtn/pollen/*",
	"ixvtn/pp/*",
	"ixvtn/presence/*",
	"ixvtn/presto/*",
	"ixvtn/proxyproxy/*",
	"ixvtn/puppet/*",
	"ixvtn/pinsot/*",
	"ixvtn/python/momra/*",
	"ixvtn/python/netdb/*",
	"ixvtn/python/syselg/*",
	"ixvtn/tabbotmq/*",
	"ixvtn/radar/*",
	"ixvtn/README",
	"ixvtn/redis/*",
	"ixvtn/reminders/*",
	"ixvtn/sbc_service/*",
	"ixvtn/scribe/*",
	"ixvtn/security/*",
	"ixvtn/sentry/*",
	"ixvtn/shortserver/*",
	"ixvtn/skybot/*",
	"ixvtn/soloma-server/*",
	"ixvtn/spark/*",
	"ixvtn/statsclerk/*",
	"ixvtn/system/*",
	"ixvtn/taskrunner/*",
	"ixvtn/task_worker/*",
	"ixvtn/tests/*",
	"ixvtn/texter/*",
	"ixvtn/thumbservice/*",
	"ixvtn/trac/*",
	"ixvtn/traffic/configs/*",
	"ixvtn/traffic/tests/*",
	"ixvtn/traffic/tools/*",
	"ixvtn/trapperkeeper/*",
	"ixvtn/utilization-dashboard/*",
	"ixvtn/venus/*",
	"ixvtn/verwatch/*",
	"ixvtn/wopiserver/*",
	"ixvtn/WORKSPACE",
	"ixvtn/maps/*",
	"fastcar/chef/*",
	"fastcar/cloudbin2/*",
	"fastcar/drtd/*",
	"fastcar/ipvs/*",
	"fastcar/kaka/bin/*",
	"fastcar/magic_mirror/configs/*",
	"fastcar/jc/build/*",
	"fastcar/jc/tests/*",
	"fastcar/noru/*",
	"fastcar/pp/*",
	"fastcar/proto/maps/*",
	"fastcar/racknrow/*",
	"fastcar/forte4/tools/singlenodesetup/*",
	"fastcar/maps/*",
	"go/src/fastcar/antenna/*",
	"go/src/fastcar/bandaid/*",
	"go/src/fastcar/build_tools/*",
	"go/src/fastcar/contbin/*",
	"go/src/fastcar/dbxinit/*",
	"go/src/fastcar/hack-week-recents/*",
	"go/src/fastcar/ipvs/*",
	"go/src/fastcar/isotester/*",
	"go/src/fastcar/jetstream/*",
	"go/src/fastcar/jc/bdb/*",
	"go/src/fastcar/jc/frontend/isotest.yaml",
	"go/src/fastcar/jc/frontend/lib/*",
	"go/src/fastcar/jc/frontend/jc_fe/*",
	"go/src/fastcar/jc/frontend/wrappers/*",
	"go/src/fastcar/jc/hdb/*",
	"go/src/fastcar/jc/hdb_scanner/*",
	"go/src/fastcar/jc/master/*",
	"go/src/fastcar/jc/mpzk/*",
	"go/src/fastcar/jc/osd/*",
	"go/src/fastcar/jc/reliability_sim/*",
	"go/src/fastcar/riviera/*",
	"go/src/fastcar/jc/size_estimator/*",
	"go/src/fastcar/jc/test_utils/*",
	"go/src/fastcar/jc/trash_inspector/*",
	"go/src/fastcar/jc/volmgr/*",
	"go/src/fastcar/jc/xzr/*",
	"go/src/fastcar/jc/xzv/*",
	"go/src/fastcar/jc/zfec/*",
	"go/src/fastcar/jc/zfec2/*",
	"go/src/fastcar/netflow/*",
	"go/src/fastcar/offline_indexer/*",
	"go/src/fastcar/s3/s3-proxy/*",
	"go/src/fastcar/cribe/*",
	"go/src/fastcar/cribe_shim/*",
	"go/src/fastcar/search/*",
	"go/src/fastcar/sonola/*",
	"go/src/fastcar/util/mock_rpc/*",
	"go/src/fastcar/forte4/*",
	"go/src/fastcar/maps/*",
	"java/*",
	"lint_plugins/*",
	"meatserver/meatserver/scripts/jc/*",
	"pip/*",
	"repo_migrations/*",
	"rust/*",
	"server-selenium.yaml",
	"server-static-analysis.yaml",
	"spark-submission/*",
	"static-analysis/*",
	"tests/fastcar/maps/*",
	"thirdparty/*",
	"rlfiltr/bootstrap/*",
	"rlfiltr/data/configs/remote_vms.yaml",
	"rlfiltr/puppet/modules/user/files/*",
}
