package terminfo_test

import (
	"fmt"
	"testing"

	"gopkg.in/check.v1"

	"gopkg.in/terminfo.v0"
)

type tiSuite struct{}

func Test(t *testing.T) { check.TestingT(t) }

var _ = check.Suite(&tiSuite{})

func (*tiSuite) TestLiteral(c *check.C) {
	buf, err := terminfo.UnescapeString("hello %{200}%d %{200}%o %{200}%x %{200}%X %'x'%d %'x'%c %{65}%c")
	c.Assert(err, check.IsNil)
	c.Check(string(buf), check.Equals, "hello 200 310 c8 C8 120 x A")
}

func (*tiSuite) TestIncrement(c *check.C) {
	buf, err := terminfo.UnescapeString("%i%p1%d", 1)
	c.Assert(err, check.IsNil)
	c.Check(string(buf), check.Equals, "2")

	buf, err = terminfo.UnescapeString("%i%p1%p2%p3%p4%d%d%d%d", 1, 2, 3, 4)
	c.Assert(err, check.IsNil)
	c.Check(string(buf), check.Equals, "4332")
}

func (*tiSuite) TestStrlen(c *check.C) {
	buf, err := terminfo.UnescapeString("%p1%l%d", "hello")
	c.Assert(err, check.IsNil)
	c.Check(string(buf), check.Equals, fmt.Sprint(len("hello")))
}

func (*tiSuite) TestMathBinop(c *check.C) {
	for _, xs := range [][2]string{
		{"+", "16"},
		{"-", "8"},
		{"*", "48"},
		{"/", "3"},
		{"m", "0"},
		{"&", "4"},
		{"|", "12"},
		{"^", "8"},
	} {
		op, result := xs[0], xs[1]
		buf, err := terminfo.UnescapeString("%p1%p2%"+op+"%d", 12, 4)
		c.Assert(err, check.IsNil, check.Commentf(op))
		c.Check(string(buf), check.Equals, result, check.Commentf(op))
	}
}

func (*tiSuite) TestMathUnary(c *check.C) {
	for _, s := range []struct {
		op  string
		arg int
		res string
	}{
		{op: "!", arg: 1, res: "0"},
		{op: "!", arg: 0, res: "1"},
		// TODO: can't find an example to check if this is right (ask curses devs?)
		{op: "~", arg: 12, res: "-13"},
	} {
		buf, err := terminfo.UnescapeString("%p1%"+s.op+"%d", s.arg)
		c.Assert(err, check.IsNil, check.Commentf(s.op))
		c.Check(string(buf), check.Equals, s.res, check.Commentf(s.op))
	}
}

func (*tiSuite) TestLogicBinop(c *check.C) {
	for _, s := range []struct {
		op  string
		arg int
		res string
	}{
		{op: "=", arg: 1, res: "0"},
		{op: "=", arg: 2, res: "1"},
		{op: "=", arg: 3, res: "0"},
		{op: "<", arg: 1, res: "1"},
		{op: "<", arg: 2, res: "0"},
		{op: "<", arg: 3, res: "0"},
		{op: ">", arg: 1, res: "0"},
		{op: ">", arg: 2, res: "0"},
		{op: ">", arg: 3, res: "1"},
	} {
		buf, err := terminfo.UnescapeString("%p1%{2}%"+s.op+"%d", s.arg)
		comment := check.Commentf("%d %s 2?", s.arg, s.op)
		c.Assert(err, check.IsNil, comment)
		c.Check(string(buf), check.Equals, s.res, comment)
	}
}

func (*tiSuite) TestLogicAndOr(c *check.C) {
	for _, b1 := range []bool{true, false} {
		var n1 int
		if b1 {
			n1 = 1
		}
		for _, b2 := range []bool{true, false} {
			var n2 int
			if b2 {
				n2 = 1
			}

			buf, err := terminfo.UnescapeString("%p1%p2%A%d", n1, n2)
			comment := check.Commentf("%t && %t", b1, b2)
			c.Assert(err, check.IsNil, comment)
			if b1 && b2 {
				c.Check(string(buf), check.Equals, "1")
			} else {
				c.Check(string(buf), check.Equals, "0")
			}

			buf, err = terminfo.UnescapeString("%p1%p2%O%d", n1, n2)
			comment = check.Commentf("%t || %t", b1, b2)
			c.Assert(err, check.IsNil, comment)
			if b1 || b2 {
				c.Check(string(buf), check.Equals, "1")
			} else {
				c.Check(string(buf), check.Equals, "0")
			}
		}
	}
}

func (*tiSuite) TestIf(c *check.C) {
	buf, err := terminfo.UnescapeString("%{1}%tYES%;")
	c.Assert(err, check.IsNil)
	c.Check(string(buf), check.Equals, "YES")

	buf, err = terminfo.UnescapeString("%{0}%tYES%;")
	c.Assert(err, check.IsNil)
	c.Check(string(buf), check.Equals, "")
}

func (*tiSuite) TestIfElse(c *check.C) {
	buf, err := terminfo.UnescapeString("%{1}%tYES%eNO%;")
	c.Assert(err, check.IsNil)
	c.Check(string(buf), check.Equals, "YES")

	buf, err = terminfo.UnescapeString("%{0}%tYES%eNO%;")
	c.Assert(err, check.IsNil)
	c.Check(string(buf), check.Equals, "NO")
}

func (*tiSuite) TestIfElseAndThenIfElse(c *check.C) {
	for i, s := range []struct {
		p1  int
		p2  int
		res string
	}{
		{1, 0, "a Y1 N2 z"},
		{1, 1, "a Y1 Y2 z"},
		{0, 1, "a N1 Y2 z"},
		{0, 0, "a N1 N2 z"},
	} {
		buf, err := terminfo.UnescapeString("a %p1%tY1%eN1%; %p2%tY2%eN2%; z", s.p1, s.p2)
		c.Assert(err, check.IsNil, check.Commentf("iter %d", i))
		c.Check(string(buf), check.Equals, s.res, check.Commentf("iter %d", i))
	}
}

func (*tiSuite) TestIfElseAlgol68(c *check.C) {
	for i, s := range []struct {
		p1  int
		p2  int
		res string
	}{
		{1, 0, "Y1"},
		{1, 1, "Y1"},
		{0, 1, "Y2"},
		{0, 0, "NO"},
	} {
		buf, err := terminfo.UnescapeString("%p1%tY1%e%p2%tY2%eNO%;", s.p1, s.p2)
		c.Assert(err, check.IsNil, check.Commentf("iter %d", i))
		c.Check(string(buf), check.Equals, s.res, check.Commentf("iter %d", i))
	}
}

func (*tiSuite) TestInitColor(c *check.C) {
	// xterm's initc
	initc := "\x1b]4;%p1%d;rgb:%p2%{255}%*%{1000}%/%2.2X/%p3%{255}%*%{1000}%/%2.2X/%p4%{255}%*%{1000}%/%2.2X\x1b\\"
	buf, err := terminfo.UnescapeString(initc, 1, 1000, 100, 80)
	c.Assert(err, check.IsNil)
	c.Check(string(buf), check.Equals, "\x1b]4;1;rgb:FF/19/14\x1b\\")

}
