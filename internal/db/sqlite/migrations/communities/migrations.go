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

var _db_sqlite_migrations_communities_1646780237_sql = []byte("\x1f\x8b\x08\x00\x00\x00\x00\x00\x00\xff\x64\xce\x41\xca\xc2\x40\x0c\xc5\xf1\xfd\x9c\xe2\x2d\xbf\x0f\xed\x09\xba\xf5\x0a\x1e\x20\x6d\x83\x04\x67\x92\x21\x49\xa9\xbd\xbd\x28\x22\xa2\xbb\x07\xef\xb7\xf8\x0f\x03\x0e\x4d\x2e\x4e\xc9\x38\xf7\x32\x3b\x3f\x56\xd2\x54\x19\xb3\xb5\xb6\xaa\xa4\x70\xe0\xaf\x00\x80\x2c\x48\xbe\x25\xba\x4b\x23\xdf\x71\xe5\x1d\x6a\x09\x5d\x6b\x3d\x3e\x45\xa7\x88\xcd\xfc\xe5\xbe\x3e\xf6\x90\x48\xd6\xc4\x64\x56\x99\xf4\x0d\xca\xff\x58\x3e\x53\x4e\xb6\x69\x59\xdc\xfa\x6f\xca\x78\x0f\x00\x00\xff\xff\x08\x34\xfa\x42\xb3\x00\x00\x00")

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
