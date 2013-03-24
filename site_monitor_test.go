package main

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func setupTestFile(filename string, contents string) (newFile *os.File, err error) {
	newFile, err = os.Create(os.TempDir() + "/" + filename);
	if err == nil {
		_, err = newFile.WriteString(contents)
		if err == nil {
			err = resetFile(newFile)
		}

	}
	return
}

/*func TestSendNotification(t *testing.T) {
	const (
		subject = "test"
		message = "site_monitor_test.go"
	)
	err := SendNotification(subject, message)
	if err != nil {
		//t.Error(err)
		t.Fail()
	}
}*/

func TestHasBuyButtonFalse(t *testing.T) {
	const (
		filename = "TestHasBuyButtonFalse.html"
		contents = "<html>\n<body>\n<h2>test</h2>\n</body>\n</html>\n"
	)

	var (
		f *os.File
		yes bool
		err error
	)

	f, err = setupTestFile(filename, contents)
	if err != nil {
		t.Error(err)
	}

	yes, err = hasBuyButton(f)
	if err != nil {
		t.Error(err)
	}
	if yes {
		t.Fail()
	}

	err = os.Remove(f.Name())
	if err != nil {
		t.Error(err)
	}
}

func TestHasBuyButtonTrue(t *testing.T) {
	const (
		filename = "TestHasBuyButtonTrue.html"
		contents = "<html>\n<body>\n<img id=\"ctl00_Imagepng3\" class=\"hand\" src=\"../../../App_Themes/Default/Images/buy-tickets.png\" alt=\"Buy Tickets\" style=\"border-width:0px;\" />\n</body>\n</html>\n"
	)

	var (
		f *os.File
		yes bool
		err error
	)

	f, err = setupTestFile(filename, contents)
	if err != nil {
		t.Error(err)
	}

	yes, err = hasBuyButton(f)
	if err != nil {
		t.Error(err)
	}
	if !yes {
		t.Fail()
	}

	err = os.Remove(f.Name())
	if err != nil {
		t.Error(err)
	}
}

func TestReadJsonFile(t *testing.T) {
	type Person struct {
		FirstName string
		LastName string
	}

	const (
		filename = "TestReadJsonFile.json"
		contents = `{
"FirstName": "Joe",
"LastName": "Schmoe"
}
`
	)

	var (
		f *os.File
		p Person
		err error
	)

	f, err = setupTestFile(filename, contents)
	if err != nil {
		t.Error(err)
	}

	err = readJsonFile(f.Name(), &p)
	if err != nil {
		t.Error(err)
	}
	if p.FirstName != "Joe" {
		t.Logf("FirstName did not match the expected 'Joe': %v", p.FirstName)
		t.Fail()
	}
	if p.LastName != "Schmoe" {
		t.Logf("LastName did not match the expected 'Schmoe': %v", p.LastName)
		t.Fail()
	}

	err = os.Remove(f.Name())
	if err != nil {
		t.Error(err)
	}
}

func TestViewstateFilter(t *testing.T) {
	const (
		contentsText = `<html>
<body>
<input id="test" type="text" value="testing"/>
<input id="__VIEWSTATE" type="hidden" value="this should be deleted"/>
<input id="__EVENTVALIDATION" type="hidden" value="this should be deleted"/>
</body>
</html>
`
		expectedText = `<html>
<body>
<input id="test" type="text" value="testing"/>
<input id="__VIEWSTATE" type="hidden" value=""/>
<input id="__EVENTVALIDATION" type="hidden" value=""/>
</body>
</html>
`
	)

	var (
		filter Filter
		contents *strings.Reader
		expected []byte
		result *bytes.Buffer
		n int64
		err error
	)

	contents = strings.NewReader(contentsText)
	expected = []byte(expectedText)
	result = bytes.NewBuffer(make([]byte, 0, len(contentsText)))

	filter, err = NewViewstateFilter(result)
	if err != nil {
		t.Log("Failed to create a new ViewstateFilter")
		t.FailNow()
	}

	n, err = filter.ReadFrom(contents)
	if err != nil {
		t.Log("Failed to read from contents and write to result")
		t.FailNow()
	}

	if n != int64(result.Len()) {
		t.Logf("Sizes don't match: written=%d ; len=%d", n, result.Len())
		t.Fail()
	}

	if bytes.Compare(result.Bytes(), expected) != 0 {
		t.Logf("Result didn't match what was expected:\n--result\n%s\n--expected\n%s",
				result.String(), expectedText)
		t.Fail()
	}
}

