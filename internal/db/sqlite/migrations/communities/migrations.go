package communities

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"strings"
)

func bindata_read(data []byte, name string) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewBuffer(data))
	if err != nil {
		return nil, fmt.Errorf("Read %q: %v", name, err)
	}

	var buf bytes.Buffer
	_, err = io.Copy(&buf, gz)
	gz.Close()

	if err != nil {
		return nil, fmt.Errorf("Read %q: %v", name, err)
	}

	return buf.Bytes(), nil
}

var _db_sqlite_migrations_communities_1646780237_sql = []byte("\x1f\x8b\x08\x00\x00\x00\x00\x00\x00\xff\x64\xcd\x41\x0a\xc2\x40\x0c\x85\xe1\x7d\x4e\xf1\x96\x8a\xf6\x04\xdd\x7a\x05\x0f\x30\xb6\x41\x82\x33\x49\xc8\xa4\xd4\xde\x5e\x14\x05\xa1\xbb\x07\xef\x87\x6f\x18\x70\x6a\x72\x8f\x92\x8c\xab\xd3\x14\xfc\x5e\x59\x6e\x95\x31\x59\x6b\x8b\x4a\x0a\x77\x1c\x08\x00\x64\x46\xf2\x33\xe1\x21\xad\xc4\x86\x07\x6f\x50\x4b\xe8\x52\xeb\xf9\x53\x78\xe9\x7d\xb5\xf8\x76\xbf\x8f\x8e\x23\xfd\x4b\x17\x5b\x95\xe6\x30\xdf\x4b\xe3\x2b\x00\x00\xff\xff\x7c\x0f\xef\x38\x92\x00\x00\x00")

func db_sqlite_migrations_communities_1646780237_sql() ([]byte, error) {
	return bindata_read(
		_db_sqlite_migrations_communities_1646780237_sql,
		"../../db/sqlite/migrations/communities/1646780237.sql",
	)
}

// Asset loads and returns the asset for the given name.
// It returns an error if the asset could not be found or
// could not be loaded.
func Asset(name string) ([]byte, error) {
	cannonicalName := strings.Replace(name, "\\", "/", -1)
	if f, ok := _bindata[cannonicalName]; ok {
		return f()
	}
	return nil, fmt.Errorf("Asset %s not found", name)
}

// AssetNames returns the names of the assets.
func AssetNames() []string {
	names := make([]string, 0, len(_bindata))
	for name := range _bindata {
		names = append(names, name)
	}
	return names
}

// _bindata is a table, holding each asset generator, mapped to its name.
var _bindata = map[string]func() ([]byte, error){
	"../../db/sqlite/migrations/communities/1646780237.sql": db_sqlite_migrations_communities_1646780237_sql,
}
// AssetDir returns the file names below a certain
// directory embedded in the file by go-bindata.
// For example if you run go-bindata on data/... and data contains the
// following hierarchy:
//     data/
//       foo.txt
//       img/
//         a.png
//         b.png
// then AssetDir("data") would return []string{"foo.txt", "img"}
// AssetDir("data/img") would return []string{"a.png", "b.png"}
// AssetDir("foo.txt") and AssetDir("notexist") would return an error
// AssetDir("") will return []string{"data"}.
func AssetDir(name string) ([]string, error) {
	node := _bintree
	if len(name) != 0 {
		cannonicalName := strings.Replace(name, "\\", "/", -1)
		pathList := strings.Split(cannonicalName, "/")
		for _, p := range pathList {
			node = node.Children[p]
			if node == nil {
				return nil, fmt.Errorf("Asset %s not found", name)
			}
		}
	}
	if node.Func != nil {
		return nil, fmt.Errorf("Asset %s not found", name)
	}
	rv := make([]string, 0, len(node.Children))
	for name := range node.Children {
		rv = append(rv, name)
	}
	return rv, nil
}

type _bintree_t struct {
	Func func() ([]byte, error)
	Children map[string]*_bintree_t
}
var _bintree = &_bintree_t{nil, map[string]*_bintree_t{
	"..": &_bintree_t{nil, map[string]*_bintree_t{
		"..": &_bintree_t{nil, map[string]*_bintree_t{
			"db": &_bintree_t{nil, map[string]*_bintree_t{
				"sqlite": &_bintree_t{nil, map[string]*_bintree_t{
					"migrations": &_bintree_t{nil, map[string]*_bintree_t{
						"communities": &_bintree_t{nil, map[string]*_bintree_t{
							"1646780237.sql": &_bintree_t{db_sqlite_migrations_communities_1646780237_sql, map[string]*_bintree_t{
							}},
						}},
					}},
				}},
			}},
		}},
	}},
}}
