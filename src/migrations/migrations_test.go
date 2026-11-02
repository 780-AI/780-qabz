package migrations

import (
	"fmt"
	"io/fs"
	"regexp"
	"sort"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
)

var name = regexp.MustCompile(`^(\d{6})_[a-z0-9_]+\.(up|down)\.sql$`)

func TestFilesArePairedAndSequential(t *testing.T) {
	entries, err := fs.ReadDir(FS, ".")
	assert.NoError(t, err)
	ups, downs := map[int]string{}, map[int]string{}
	for _, e := range entries {
		m := name.FindStringSubmatch(e.Name())
		if !assert.NotNil(t, m, "bad file name %s", e.Name()) {
			continue
		}
		n, _ := strconv.Atoi(m[1])
		if m[2] == "up" {
			ups[n] = e.Name()
		} else {
			downs[n] = e.Name()
		}
	}
	assert.NotEmpty(t, ups)
	var nums []int
	for n := range ups {
		nums = append(nums, n)
		assert.Contains(t, downs, n, fmt.Sprintf("%06d has no down file", n))
	}
	assert.Len(t, downs, len(ups))
	sort.Ints(nums)
	for i, n := range nums {
		assert.Equal(t, i+1, n, "versions must be 1,2,3 without gaps")
	}
}

func TestAppHotConfigsColumns(t *testing.T) {
	b, err := fs.ReadFile(FS, "000001_app_hot_configs.up.sql")
	assert.NoError(t, err)
	for _, want := range []string{"id         serial", "key        varchar(256)", "cnf_type   varchar(64)",
		"value      text", "created_at timestamp", "updated_at timestamp"} {
		assert.Contains(t, string(b), want)
	}
}
