package operation_setting

// TOKEN fork: do not point customers at New API's own documentation by
// default. Empty hides the Docs link until TOKEN's docs URL is configured in
// System Settings (general_setting.docs_link).
func init() {
	generalSetting.DocsLink = ""
}
