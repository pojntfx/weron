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

var _db_sqlite_migrations_communities_1646780237_sql = []byte("\x1f\x8b\x08\x00\x00\x00\x00\x00\x00\xff\x64\xce\x4b\xaa\xc3\x30\x0c\x85\xe1\xb9\x57\x71\x86\xf7\xd2\x66\x05\x99\x76\x0b\x5d\x80\xe3\x88\x20\x6a\x4b\x46\x52\x48\xb3\xfb\xd2\x52\x4a\x1f\x33\x81\xbe\x03\xff\x30\xe0\xd0\x78\xb1\x1c\x84\x73\x4f\xc5\xe8\x7e\x45\x9e\x2a\xa1\x68\x6b\xab\x70\x30\x39\xfe\x12\x00\xf0\x8c\xa0\x6b\xa0\x1b\xb7\x6c\x3b\x2e\xb4\x43\x34\x20\x6b\xad\xc7\x87\xe8\xd9\x7d\x53\x7b\xba\xcf\x5f\xa9\x4c\x12\x0e\x96\xa0\x85\xec\x7b\x49\xe6\xec\x41\x12\x98\x54\x2b\x65\x79\x81\xf4\x3f\xa6\xf7\xd0\x93\x6e\x92\x66\xd3\xfe\x1b\x3a\xde\x02\x00\x00\xff\xff\xea\x26\xc1\x9c\xd1\x00\x00\x00")

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
