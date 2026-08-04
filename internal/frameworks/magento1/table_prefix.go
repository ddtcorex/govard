package magento1

import (
	"encoding/xml"
	"os"
	"path/filepath"

	"govard/internal/engine"
)

// DetectTablePrefix reads the Magento 1 local.xml table-prefix setting.
// OpenMage inherits this detector through its resolved Magento 1 definition.
func DetectTablePrefix(root string) string {
	data, err := os.ReadFile(filepath.Join(root, "app", "etc", "local.xml"))
	if err != nil {
		return ""
	}

	var localXML struct {
		Global struct {
			Resources struct {
				DB struct {
					TablePrefix string `xml:"table_prefix"`
				} `xml:"db"`
			} `xml:"resources"`
		} `xml:"global"`
	}
	if err := xml.Unmarshal(data, &localXML); err != nil {
		return ""
	}
	return engine.NormalizeTablePrefix(localXML.Global.Resources.DB.TablePrefix)
}
