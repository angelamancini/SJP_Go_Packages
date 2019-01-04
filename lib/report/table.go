package report

import (
	"github.com/olekukonko/tablewriter"
	"os"
)

//any struct that implements these two functions can use the table output function
type Table interface {
	TableData() [][]string
	TableHeaders() []string
}

//type TableData [][]string

func OutputTable(t Table) {
	table := tablewriter.NewWriter(os.Stdout)
	table.SetAutoWrapText(false)
	table.SetHeader(t.TableHeaders())
	table.AppendBulk(t.TableData())
	table.Render()
}

func DrawTable(headers []string, data [][]string) {
	table := tablewriter.NewWriter(os.Stdout)
	table.SetAutoWrapText(false)
	table.SetHeader(headers)
	table.AppendBulk(data)
	table.Render()
}
