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

var _db_sqlite_migrations_communities_1646780237_sql = []byte("\x1f\x8b\x08\x00\x00\x00\x00\x00\x00\xff\x64\x8e\x41\xca\xc2\x40\x0c\x46\xf7\x39\xc5\xb7\xfc\x7f\xb4\x27\xe8\xd6\x2b\x78\x80\x69\x1b\x24\x38\x93\x0c\x49\x4a\xed\xed\x45\x11\x11\xdd\x3d\x78\x6f\xf1\x86\x01\x87\x26\x17\x2f\xc9\x38\x77\x9a\x9d\x1f\x94\x65\xaa\x8c\xd9\x5a\x5b\x55\x52\x38\xf0\x47\x00\x20\x0b\x92\x6f\x89\xee\xd2\x8a\xef\xb8\xf2\x0e\xb5\x84\xae\xb5\x1e\x9f\x45\x2f\x11\x9b\xf9\xab\xfb\x72\xec\x21\x91\xac\x89\xc9\xac\xbe\x2d\xfd\x8f\xf4\xf9\x71\xb2\x4d\x69\x71\xeb\xbf\x1f\xe3\x3d\x00\x00\xff\xff\x1d\xe8\xba\x67\xb0\x00\x00\x00")

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
