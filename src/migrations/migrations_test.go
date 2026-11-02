package migrations

import (
	"io/fs"
	"regexp"
	"sort"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
)

var name = regexp.MustCompile(`^(\d{6})_[a-z0-9_]+\.up\.sql$`)

func TestFilesAreUpOnlyAndSequential(t *testing.T) {
	entries, err := fs.ReadDir(FS, ".")
	assert.NoError(t, err)
	var nums []int
	for _, e := range entries {
		m := name.FindStringSubmatch(e.Name())
		if !assert.NotNil(t, m, "bad file name %s (only NNNNNN_name.up.sql is allowed; no down files)", e.Name()) {
			continue
		}
		n, _ := strconv.Atoi(m[1])
		nums = append(nums, n)
	}
	assert.NotEmpty(t, nums)
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
