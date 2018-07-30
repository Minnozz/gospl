package scanner

import (
	"github.com/Minnozz/gompiler/token"
)

type Scanner struct {
	fileInfo *token.FileInfo
	src      []byte

	ch     byte
	offset int
}

func (s *Scanner) Init(fileInfo *token.FileInfo, src []byte) {
	s.fileInfo = fileInfo
	s.src = src
	s.ch = 0
	s.offset = -1 // Advance to 0 with first call to next()

	// Read first byte
	s.next()
}

func (s *Scanner) Scan() (pos int, tok token.Token, lit string) {
	s.skipWhitespace()

	// Record start of token
	pos = s.offset

	// Determine token by looking at the first character
	switch ch := s.ch; {
	case s.offset >= len(s.src):
		tok = token.EOF
	case isAlpha(ch):
		lit = s.scanWord()
		tok, lit = token.LookupWord(lit)
	case isDigit(ch):
		tok = token.INTEGER
		lit = s.scanNumber()
	default:
		// First advance to the next character
		s.next()
		// Then look at the current(/previous) character
		switch ch {
		case '+':
			tok = token.PLUS
		case '-':
			tok = token.MINUS
		case '*':
			tok = token.MULTIPLY
		case '/':
			if s.ch == '/' || s.ch == '*' {
				tok = token.COMMENT
				lit = s.scanComment()
			} else {
				tok = token.DIVIDE
			}
		case '%':
			tok = token.MODULO
		case '&':
			tok = s.expect('&', token.AND)
		case '|':
			tok = s.expect('|', token.OR)
		case '=':
			tok = s.try('=', token.EQUALS, token.IS)
		case '<':
			tok = s.try('=', token.LESS_THAN_EQUALS, token.LESS_THAN)
		case '>':
			tok = s.try('=', token.GREATER_THAN_EQUALS, token.GREATER_THAN)
		case '!':
			tok = s.try('=', token.NOT_EQUALS, token.NOT)
		case ',':
			tok = token.COMMA
		case ';':
			tok = token.SEMICOLON
		case ':':
			tok = token.COLON
		case '(':
			tok = token.ROUND_BRACKET_OPEN
		case ')':
			tok = token.ROUND_BRACKET_CLOSE
		case '{':
			tok = token.CURLY_BRACKET_OPEN
		case '}':
			tok = token.CURLY_BRACKET_CLOSE
		case '[':
			tok = token.SQUARE_BRACKET_OPEN
		case ']':
			tok = token.SQUARE_BRACKET_CLOSE
		default:
			// TODO: error
			tok, lit = token.INVALID, string(ch)
		}
	}

	return pos, tok, lit
}

func (s *Scanner) next() {
	s.offset++
	if s.offset < len(s.src) {
		s.ch = s.src[s.offset]
		if s.ch == '\n' {
			s.fileInfo.AddLine(s.offset)
		}
	} else {
		s.ch = 0
	}
}

func (s *Scanner) skipWhitespace() {
	for s.ch == ' ' || s.ch == '\t' || s.ch == '\n' || s.ch == '\r' {
		s.next()
	}
}

func (s *Scanner) scanWord() string {
	start := s.offset
	for isWord(s.ch) {
		s.next()
	}
	return string(s.src[start:s.offset])
}

func (s *Scanner) scanNumber() string {
	start := s.offset
	for isDigit(s.ch) {
		s.next()
	}
	return string(s.src[start:s.offset])
}

func (s *Scanner) scanComment() string {
	// Initial '/' has been consumed; s.ch is the next character
	start := s.offset - 1

	if s.ch == '/' {
		// Line comment
		s.next()
		for s.ch != '\n' && s.ch != 0 {
			s.next()
		}
		goto ok
	}

	// Block comment
	s.next()
	for s.ch != 0 {
		ch := s.ch
		s.next()
		if ch == '*' && s.ch == '/' {
			s.next()
			goto ok
		}
		// TODO: error comment not terminated
	}
ok:
	return string(s.src[start:s.offset])
}

func (s *Scanner) expect(ch byte, match token.Token) token.Token {
	if s.ch == ch {
		s.next()
		return match
	}
	// TODO: Error
	return token.INVALID
}

func (s *Scanner) try(ch byte, match, mismatch token.Token) token.Token {
	if s.ch == ch {
		s.next()
		return match
	}
	return mismatch
}
