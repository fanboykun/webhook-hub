package renderprofile

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"text/template"
	"text/template/parse"
	"time"

	"github.com/fanboykun/webhook-hub/internal/domain"
	"github.com/fanboykun/webhook-hub/internal/eventcatalog"
)

const MaxTemplateBytes = 64 << 10

type Context struct {
	Source      string
	EventKey    string
	EventType   string
	Title       string
	Summary     string
	Severity    string
	Lifecycle   string
	Service     string
	Environment string
	SourceURL   string
	OccurredAt  time.Time
	Payload     map[string]any
}

type CompiledTemplate struct {
	template *template.Template
}

type CompiledPair struct {
	Title *CompiledTemplate
	Body  *CompiledTemplate
}

type CompiledProfile struct {
	Source   domain.Source
	Key      string
	Version  int
	Slack    *CompiledPair
	Telegram *CompiledTemplate
	Teams    *CompiledPair
	Email    *CompiledPair
}

func CompileProfile(profile domain.RendererProfile, definition eventcatalog.Definition) (CompiledProfile, error) {
	if profile.Source != definition.Source || profile.Key != string(definition.Key) {
		return CompiledProfile{}, fmt.Errorf("profile contract %s:%s does not match definition %s:%s", profile.Source, profile.Key, definition.Source, definition.Key)
	}
	if profile.Templates.Slack == nil && profile.Templates.Telegram == nil && profile.Templates.Teams == nil && profile.Templates.Email == nil {
		return CompiledProfile{}, errors.New("at least one destination template is required")
	}

	out := CompiledProfile{Source: profile.Source, Key: profile.Key, Version: definition.PayloadVersion}
	var errs []error
	if profile.Templates.Slack != nil {
		title, err := CompileTemplate("slack_title", profile.Templates.Slack.Title, definition)
		if err != nil {
			errs = append(errs, fmt.Errorf("slack title: %w", err))
		}
		body, err := CompileTemplate("slack_body", profile.Templates.Slack.Body, definition)
		if err != nil {
			errs = append(errs, fmt.Errorf("slack body: %w", err))
		}
		out.Slack = &CompiledPair{Title: title, Body: body}
	}
	if profile.Templates.Telegram != nil {
		text, err := CompileTemplate("telegram_text", profile.Templates.Telegram.Text, definition)
		if err != nil {
			errs = append(errs, fmt.Errorf("telegram text: %w", err))
		}
		out.Telegram = text
	}
	if profile.Templates.Teams != nil {
		title, err := CompileTemplate("teams_title", profile.Templates.Teams.Title, definition)
		if err != nil {
			errs = append(errs, fmt.Errorf("teams title: %w", err))
		}
		body, err := CompileTemplate("teams_body", profile.Templates.Teams.Body, definition)
		if err != nil {
			errs = append(errs, fmt.Errorf("teams body: %w", err))
		}
		out.Teams = &CompiledPair{Title: title, Body: body}
	}
	if profile.Templates.Email != nil {
		subject, err := CompileTemplate("email_subject", profile.Templates.Email.Subject, definition)
		if err != nil {
			errs = append(errs, fmt.Errorf("email subject: %w", err))
		}
		body, err := CompileTemplate("email_body", profile.Templates.Email.Body, definition)
		if err != nil {
			errs = append(errs, fmt.Errorf("email body: %w", err))
		}
		out.Email = &CompiledPair{Title: subject, Body: body}
	}
	if err := errors.Join(errs...); err != nil {
		return CompiledProfile{}, err
	}
	return out, nil
}

func CompileTemplate(name, source string, definition eventcatalog.Definition) (*CompiledTemplate, error) {
	if source == "" {
		return nil, nil
	}
	if len(source) > MaxTemplateBytes {
		return nil, fmt.Errorf("template exceeds %d bytes", MaxTemplateBytes)
	}
	tmpl, err := template.New(name).Option("missingkey=error").Parse(source)
	if err != nil {
		return nil, fmt.Errorf("invalid template syntax: %w", err)
	}
	if len(tmpl.Templates()) != 1 {
		return nil, errors.New("associated template definitions are not allowed")
	}
	fields := make(map[string]eventcatalog.Field, len(definition.Fields))
	for _, field := range definition.Fields {
		fields[field.Path] = field
	}
	if err := validateNode(tmpl.Tree.Root, fields); err != nil {
		return nil, err
	}
	return &CompiledTemplate{template: tmpl}, nil
}

func Execute(compiled *CompiledTemplate, context Context) (string, error) {
	if compiled == nil || compiled.template == nil {
		return "", nil
	}
	var output bytes.Buffer
	if err := compiled.template.Execute(&output, context); err != nil {
		return "", err
	}
	return output.String(), nil
}

func validateNode(node parse.Node, fields map[string]eventcatalog.Field) error {
	if node == nil {
		return nil
	}
	switch n := node.(type) {
	case *parse.ListNode:
		for _, child := range n.Nodes {
			if err := validateNode(child, fields); err != nil {
				return err
			}
		}
		return nil
	case *parse.TextNode, *parse.StringNode, *parse.NumberNode, *parse.BoolNode, *parse.NilNode:
		return nil
	case *parse.ActionNode:
		return validateNode(n.Pipe, fields)
	case *parse.IfNode:
		if err := validateNode(n.Pipe, fields); err != nil {
			return err
		}
		if err := validateNode(n.List, fields); err != nil {
			return err
		}
		return validateNode(n.ElseList, fields)
	case *parse.PipeNode:
		if len(n.Decl) > 0 {
			return errors.New("template variables are not allowed")
		}
		for _, command := range n.Cmds {
			if err := validateNode(command, fields); err != nil {
				return err
			}
		}
		return nil
	case *parse.CommandNode:
		for _, argument := range n.Args {
			if err := validateNode(argument, fields); err != nil {
				return err
			}
		}
		return nil
	case *parse.FieldNode:
		return validateField(n.Ident, fields)
	case *parse.ChainNode:
		field, ok := n.Node.(*parse.FieldNode)
		if !ok {
			return errors.New("template chains must start from an allowed field")
		}
		return validateField(append(append([]string(nil), field.Ident...), n.Field...), fields)
	case *parse.IdentifierNode:
		return fmt.Errorf("template function %q is not allowed", n.Ident)
	case *parse.VariableNode:
		return errors.New("template variables are not allowed")
	case *parse.DotNode:
		return errors.New("whole-context access is not allowed")
	case *parse.RangeNode:
		return errors.New("template ranges are not allowed")
	case *parse.WithNode:
		return errors.New("template with blocks are not allowed")
	case *parse.TemplateNode:
		return errors.New("associated template calls are not allowed")
	default:
		return fmt.Errorf("template construct %T is not allowed", node)
	}
}

func validateField(ident []string, fields map[string]eventcatalog.Field) error {
	if len(ident) == 0 {
		return errors.New("empty template field is not allowed")
	}
	if ident[0] == "Payload" {
		if len(ident) == 1 {
			return errors.New("whole-payload access is not allowed")
		}
		path := strings.Join(ident[1:], ".")
		field, ok := fields[path]
		if !ok {
			return fmt.Errorf("payload field %q is not declared by this event definition", path)
		}
		if field.Type == eventcatalog.FieldTypeObject {
			return fmt.Errorf("payload object %q cannot be rendered directly; select a declared scalar field", path)
		}
		return nil
	}
	if ident[0] == "Metadata" {
		return errors.New("metadata is not part of the typed renderer contract")
	}
	if len(ident) == 2 && ident[0] == "OccurredAt" && ident[1] == "Format" {
		return nil
	}
	if len(ident) != 1 {
		return fmt.Errorf("template field chain %q is not allowed", strings.Join(ident, "."))
	}
	switch ident[0] {
	case "Source", "EventKey", "EventType", "Title", "Summary", "Severity", "Lifecycle", "Service", "Environment", "SourceURL", "OccurredAt":
		return nil
	default:
		return fmt.Errorf("template field %q is not available in renderer context", ident[0])
	}
}
