// Package examples holds the gallery's examples: each is a .vuka.html file
// compiled beside it (go generate), so the source shown is the source that
// renders.
package examples

import (
	"embed"

	"github.com/vuka-lang/ui/v2"
)

//go:embed *.vuka.html
var sources embed.FS

// Source is the text of an example's file.
func Source(file string) string {
	b, err := sources.ReadFile(file)
	if err != nil {
		panic("examples: no " + file)
	}
	return string(b)
}

// Files are the example files there are.
func Files() []string {
	entries, _ := sources.ReadDir(".")
	var out []string
	for _, e := range entries {
		out = append(out, e.Name())
	}
	return out
}

// Example is one example of a component: the first of a component's is
// its page's main example, the others its sections.
type Example struct {
	// Component is the kit's Go name: Button, or Icon for a function.
	Component string
	// Section is the section's name (Variants, Sizes…); "" for the main one.
	Section string
	// Heading is one sentence saying what the section shows.
	Heading string
	// File is the example's .vuka.html file.
	File string
	// Height is the preview's height in pixels when its content would not
	// size it (an overlay); 0 sizes it to the content.
	Height int
	// Node renders the example.
	Node ui.Node
}

// All are the examples, a component's in the order its page shows them.
var All = []Example{
	{Component: "Alert", File: "alert_variants.vuka.html", Node: AlertVariants{}},
	{Component: "Badge", File: "badge_variants.vuka.html", Node: BadgeVariants{}},
	{Component: "Button", File: "button_demo.vuka.html", Node: ButtonDemo{}},
	{Component: "Button", Section: "Variants", Heading: "A variant sets the tone.", File: "button_variants.vuka.html", Node: ButtonVariants{}},
	{Component: "Button", Section: "Sizes", Heading: "Small, medium (the default) and large.", File: "button_sizes.vuka.html", Node: ButtonSizes{}},
	{Component: "Button", Section: "With icon", Heading: "An icon before the label, or an icon on its own with a title.", File: "button_icon.vuka.html", Node: ButtonIcon{}},
	{Component: "Button", Section: "States", Heading: "Loading shows a spinner, disabled refuses clicks, href makes a link.", File: "button_states.vuka.html", Node: ButtonStates{}},
	{Component: "Submit", File: "submit_demo.vuka.html", Node: SubmitDemo{}},
	{Component: "Card", File: "card_demo.vuka.html", Node: CardDemo{}},
	{Component: "Skeleton", File: "skeleton_demo.vuka.html", Node: SkeletonDemo{}},
	{Component: "Image", File: "image_demo.vuka.html", Node: ImageDemo{}},
	{Component: "Icon", File: "icon_demo.vuka.html", Node: IconDemo{}},
	{Component: "Spinner", File: "spinner_demo.vuka.html", Node: SpinnerDemo{}},
	{Component: "Actions", File: "actions_demo.vuka.html", Node: ActionsDemo{}},
	{Component: "Modal", File: "modal_demo.vuka.html", Height: 360, Node: ModalDemo{}},
	{Component: "Dialog", File: "dialog_demo.vuka.html", Height: 260, Node: DialogDemo{}},
	{Component: "Drawer", File: "drawer_demo.vuka.html", Height: 320, Node: DrawerDemo{}},
	{Component: "Popover", File: "popover_demo.vuka.html", Node: PopoverDemo{}},
	{Component: "Notice", File: "notice_demo.vuka.html", Height: 160, Node: NoticeDemo{}},
	{Component: "ConfirmBox", File: "confirm_demo.vuka.html", Height: 240, Node: ConfirmDemo{}},
	{Component: "Tabs", File: "tabs_demo.vuka.html", Node: TabsDemo{}},
	{Component: "Tile", File: "tile_demo.vuka.html", Node: TileDemo{}},
	{Component: "Toast", File: "toast_demo.vuka.html", Height: 240, Node: ToastDemo{}},
	{Component: "Form", File: "form_demo.vuka.html", Node: FormDemo{}},
	{Component: "Input", File: "input_demo.vuka.html", Node: InputDemo{}},
	{Component: "Password", File: "password_demo.vuka.html", Node: PasswordDemo{}},
	{Component: "Textarea", File: "textarea_demo.vuka.html", Node: TextareaDemo{}},
	{Component: "Select", File: "select_demo.vuka.html", Node: SelectDemo{}},
	{Component: "Checkbox", File: "checkbox_demo.vuka.html", Node: CheckboxDemo{}},
	{Component: "Field", File: "field_demo.vuka.html", Node: FieldDemo{}},
	{Component: "Table", File: "table_demo.vuka.html", Node: TableDemo{}},
	{Component: "DataTable", File: "datatable_demo.vuka.html", Node: DataTableDemo{}},
}
