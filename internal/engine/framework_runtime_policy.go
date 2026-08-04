package engine

import "strings"

var nodeImageFlavors = map[string]string{}
var varnishTemplateFrameworks = map[string]string{}

func RegisterNodeImageFlavor(framework, flavor string) {
	nodeImageFlavors[strings.ToLower(strings.TrimSpace(framework))] = strings.TrimSpace(flavor)
}

func NodeImageFlavorForFramework(framework string) string {
	return nodeImageFlavors[strings.ToLower(strings.TrimSpace(framework))]
}

func RegisterVarnishTemplateFramework(framework, templateFramework string) {
	varnishTemplateFrameworks[strings.ToLower(strings.TrimSpace(framework))] = strings.ToLower(strings.TrimSpace(templateFramework))
}

func VarnishTemplateFrameworkForFramework(framework string) string {
	return varnishTemplateFrameworks[strings.ToLower(strings.TrimSpace(framework))]
}
