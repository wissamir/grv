package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
)

const (
	setCommand       = "set"
	themeCommand     = "theme"
	mapCommand       = "map"
	unmapCommand     = "unmap"
	quitCommand      = "q"
	addtabCommand    = "addtab"
	removetabCommand = "rmtab"
	addviewCommand   = "addview"
	vsplitCommand    = "vsplit"
	hsplitCommand    = "hsplit"
	splitCommand     = "split"
)

type commandConstructor func(parser *ConfigParser, commandToken *ConfigToken, tokens []*ConfigToken) (ConfigCommand, error)

// ConfigCommand represents a config command
type ConfigCommand interface {
	configCommand()
}

// SetCommand contains state for setting a config variable to a value
type SetCommand struct {
	variable *ConfigToken
	value    *ConfigToken
}

func (setCommand *SetCommand) configCommand() {}

// ThemeCommand contains state for setting a components values for on a theme
type ThemeCommand struct {
	name      *ConfigToken
	component *ConfigToken
	bgcolor   *ConfigToken
	fgcolor   *ConfigToken
}

func (themeCommand *ThemeCommand) configCommand() {}

// MapCommand contains state for mapping a key sequence to another
type MapCommand struct {
	view *ConfigToken
	from *ConfigToken
	to   *ConfigToken
}

func (mapCommand *MapCommand) configCommand() {}

// UnmapCommand contains state for unmapping a key sequence
type UnmapCommand struct {
	view *ConfigToken
	from *ConfigToken
}

func (unmapCommand *UnmapCommand) configCommand() {}

// QuitCommand represents the command to quit grv
type QuitCommand struct{}

func (quitCommand *QuitCommand) configCommand() {}

// NewTabCommand represents the command to create a new tab
type NewTabCommand struct {
	tabName *ConfigToken
}

func (newTabCommand *NewTabCommand) configCommand() {}

// RemoveTabCommand represents the command to remove the currently active tab
type RemoveTabCommand struct{}

func (removeTabCommand *RemoveTabCommand) configCommand() {}

// AddViewCommand represents the command to add a new view
// to the currently active view
type AddViewCommand struct {
	view *ConfigToken
	args []*ConfigToken
}

func (addViewCommand *AddViewCommand) configCommand() {}

// SplitViewCommand represents the command split the currently
// active view with a new view
type SplitViewCommand struct {
	orientation ContainerOrientation
	view        *ConfigToken
	args        []*ConfigToken
}

func (splitViewCommand *SplitViewCommand) configCommand() {}

type commandDescriptor struct {
	tokenTypes  []ConfigTokenType
	varArgs     bool
	constructor commandConstructor
}

var commandDescriptors = map[string]*commandDescriptor{
	setCommand: {
		tokenTypes:  []ConfigTokenType{CtkWord, CtkWord},
		constructor: setCommandConstructor,
	},
	themeCommand: {
		tokenTypes:  []ConfigTokenType{CtkOption, CtkWord, CtkOption, CtkWord, CtkOption, CtkWord, CtkOption, CtkWord},
		constructor: themeCommandConstructor,
	},
	mapCommand: {
		tokenTypes:  []ConfigTokenType{CtkWord, CtkWord, CtkWord},
		constructor: mapCommandConstructor,
	},
	unmapCommand: {
		tokenTypes:  []ConfigTokenType{CtkWord, CtkWord},
		constructor: unmapCommandConstructor,
	},
	quitCommand: {
		constructor: quitCommandConstructor,
	},
	addtabCommand: {
		tokenTypes:  []ConfigTokenType{CtkWord},
		constructor: newTabCommandConstructor,
	},
	removetabCommand: {
		constructor: newRemoveTabCommandConstructor,
	},
	addviewCommand: {
		varArgs:     true,
		constructor: addViewCommandConstructor,
	},
	vsplitCommand: {
		varArgs:     true,
		constructor: splitViewCommandConstructor,
	},
	hsplitCommand: {
		varArgs:     true,
		constructor: splitViewCommandConstructor,
	},
	splitCommand: {
		varArgs:     true,
		constructor: splitViewCommandConstructor,
	},
}

// ConfigParser is a component capable of parsing config into commands
type ConfigParser struct {
	scanner     *ConfigScanner
	inputSource string
}

// NewConfigParser creates a new ConfigParser which will read input from the provided reader
func NewConfigParser(reader io.Reader, inputSource string) *ConfigParser {
	return &ConfigParser{
		scanner:     NewConfigScanner(reader),
		inputSource: inputSource,
	}
}

// Parse returns the next command from the input stream
// eof is set to true if the end of the input stream has been reached
func (parser *ConfigParser) Parse() (command ConfigCommand, eof bool, err error) {
	var token *ConfigToken

	for {
		token, err = parser.scan()
		if err != nil {
			return
		}

		switch token.tokenType {
		case CtkWord:
			command, eof, err = parser.parseCommand(token)
		case CtkTerminator:
			continue
		case CtkEOF:
			eof = true
		case CtkOption:
			err = parser.generateParseError(token, "Unexpected Option \"%v\"", token.value)
		case CtkInvalid:
			err = parser.generateParseError(token, "Syntax Error")
		default:
			err = parser.generateParseError(token, "Unexpected token \"%v\"", token.value)
		}

		break
	}

	if err != nil {
		parser.discardTokensUntilNextCommand()
	}

	return
}

// InputSource returns the text description of the input source
func (parser *ConfigParser) InputSource() string {
	return parser.inputSource
}

func (parser *ConfigParser) scan() (token *ConfigToken, err error) {
	for {
		token, err = parser.scanner.Scan()
		if err != nil {
			return
		}

		if token.tokenType != CtkWhiteSpace && token.tokenType != CtkComment {
			break
		}
	}

	return
}

func (parser *ConfigParser) generateParseError(token *ConfigToken, errorMessage string, args ...interface{}) error {
	return generateConfigError(parser.inputSource, token, errorMessage, args...)
}

func generateConfigError(inputSource string, token *ConfigToken, errorMessage string, args ...interface{}) error {
	var buffer bytes.Buffer

	if inputSource != "" {
		buffer.WriteString(inputSource)
		buffer.WriteRune(':')
		buffer.WriteString(fmt.Sprintf("%v:%v ", token.startPos.line, token.startPos.col))
	}

	buffer.WriteString(fmt.Sprintf(errorMessage, args...))

	if token.err != nil {
		buffer.WriteString(": ")
		buffer.WriteString(token.err.Error())
	}

	return errors.New(buffer.String())
}

func (parser *ConfigParser) discardTokensUntilNextCommand() {
	for {
		token, err := parser.scan()

		if err != nil ||
			token.tokenType == CtkTerminator ||
			token.tokenType == CtkEOF {
			return
		}
	}
}

func (parser *ConfigParser) parseCommand(commandToken *ConfigToken) (command ConfigCommand, eof bool, err error) {
	commandDescriptor, ok := commandDescriptors[commandToken.value]
	if !ok {
		err = parser.generateParseError(commandToken, "Invalid command \"%v\"", commandToken.value)
		return
	}

	if commandDescriptor.varArgs {
		return parser.parseVarArgsCommand(commandDescriptor, commandToken)
	}

	var tokens []*ConfigToken

	for i := 0; i < len(commandDescriptor.tokenTypes); i++ {
		var token *ConfigToken
		token, err = parser.scan()
		expectedConfigTokenType := commandDescriptor.tokenTypes[i]

		switch {
		case err != nil:
			return
		case token.err != nil:
			err = parser.generateParseError(token, "Syntax Error")
			return
		case token.tokenType == CtkEOF:
			err = parser.generateParseError(token, "Unexpected EOF")
			eof = true
			return
		case token.tokenType != expectedConfigTokenType:
			err = parser.generateParseError(token, "Expected %v but got %v: \"%v\"",
				ConfigTokenName(expectedConfigTokenType), ConfigTokenName(token.tokenType), token.value)
			return
		}

		tokens = append(tokens, token)
	}

	command, err = commandDescriptor.constructor(parser, commandToken, tokens)

	return
}

func (parser *ConfigParser) parseVarArgsCommand(commandDescriptor *commandDescriptor, commandToken *ConfigToken) (command ConfigCommand, eof bool, err error) {
	var tokens []*ConfigToken

OuterLoop:
	for {
		var token *ConfigToken
		token, err = parser.scan()

		switch {
		case err != nil:
			return
		case token.err != nil:
			err = parser.generateParseError(token, "Syntax Error")
			return
		case token.tokenType == CtkEOF:
			break OuterLoop
		case token.tokenType == CtkTerminator:
			break OuterLoop
		}

		tokens = append(tokens, token)
	}

	command, err = commandDescriptor.constructor(parser, commandToken, tokens)
	return
}

func setCommandConstructor(parser *ConfigParser, commandToken *ConfigToken, tokens []*ConfigToken) (ConfigCommand, error) {
	return &SetCommand{
		variable: tokens[0],
		value:    tokens[1],
	}, nil
}

func themeCommandConstructor(parser *ConfigParser, commandToken *ConfigToken, tokens []*ConfigToken) (ConfigCommand, error) {
	themeCommand := &ThemeCommand{}

	optionSetters := map[string]func(*ConfigToken){
		"--name":      func(name *ConfigToken) { themeCommand.name = name },
		"--component": func(component *ConfigToken) { themeCommand.component = component },
		"--bgcolor":   func(bgcolor *ConfigToken) { themeCommand.bgcolor = bgcolor },
		"--fgcolor":   func(fgcolor *ConfigToken) { themeCommand.fgcolor = fgcolor },
	}

	for i := 0; i+1 < len(tokens); i += 2 {
		optionToken := tokens[i]
		valueToken := tokens[i+1]

		optionSetter, ok := optionSetters[optionToken.value]
		if !ok {
			return nil, parser.generateParseError(optionToken, "Invalid option for theme command: \"%v\"", optionToken.value)
		}

		optionSetter(valueToken)
	}

	return themeCommand, nil
}

func mapCommandConstructor(parser *ConfigParser, commandToken *ConfigToken, tokens []*ConfigToken) (ConfigCommand, error) {
	return &MapCommand{
		view: tokens[0],
		from: tokens[1],
		to:   tokens[2],
	}, nil
}

func unmapCommandConstructor(parser *ConfigParser, commandToken *ConfigToken, tokens []*ConfigToken) (ConfigCommand, error) {
	return &UnmapCommand{
		view: tokens[0],
		from: tokens[1],
	}, nil
}

func quitCommandConstructor(parser *ConfigParser, commandToken *ConfigToken, tokens []*ConfigToken) (ConfigCommand, error) {
	return &QuitCommand{}, nil
}

func newTabCommandConstructor(parser *ConfigParser, commandToken *ConfigToken, tokens []*ConfigToken) (ConfigCommand, error) {
	return &NewTabCommand{
		tabName: tokens[0],
	}, nil
}

func newRemoveTabCommandConstructor(parser *ConfigParser, commandToken *ConfigToken, tokens []*ConfigToken) (ConfigCommand, error) {
	return &RemoveTabCommand{}, nil
}

func addViewCommandConstructor(parser *ConfigParser, commandToken *ConfigToken, tokens []*ConfigToken) (ConfigCommand, error) {
	if len(tokens) < 1 {
		addViewCommand := commandToken.value
		return nil, parser.generateParseError(commandToken, "Invalid %[1]v command. Usage: %[1]v [VIEW] [ARGS...]", addViewCommand)
	}

	return &AddViewCommand{
		view: tokens[0],
		args: tokens[1:],
	}, nil
}

func splitViewCommandConstructor(parser *ConfigParser, commandToken *ConfigToken, tokens []*ConfigToken) (ConfigCommand, error) {
	splitViewCommand := commandToken.value

	if len(tokens) < 1 {
		return nil, parser.generateParseError(commandToken, "Invalid %[1]v command. Usage: %[1]v [VIEW] [ARGS...]", splitViewCommand)
	}

	var orientation ContainerOrientation

	switch splitViewCommand {
	case splitCommand:
		orientation = CoDynamic
	case hsplitCommand:
		orientation = CoHorizontal
	case vsplitCommand:
		orientation = CoVertical
	default:
		return nil, parser.generateParseError(commandToken, "Unrecognised command: %v", splitViewCommand)
	}

	return &SplitViewCommand{
		orientation: orientation,
		view:        tokens[0],
		args:        tokens[1:],
	}, nil
}
