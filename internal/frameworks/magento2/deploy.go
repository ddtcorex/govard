package magento2

// BuildDeployLocalesQuery owns Magento 2's locale discovery query.
func BuildDeployLocalesQuery(tablePrefix string) string {
	return "SELECT DISTINCT value FROM " + tablePrefix + "core_config_data WHERE path IN ('general/locale/code','general/locale/timezone') AND value REGEXP '^[a-z]{2}_[A-Z]{2}$';"
}
