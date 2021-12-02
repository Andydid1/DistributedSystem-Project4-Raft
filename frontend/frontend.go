package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"gw1035/project4/frontend/models"
	"net/rpc"
	"strconv"
	"strings"

	"github.com/kataras/iris/v12"
)

var (
	FrontEndPort   int
	BackEndUrls    []string
	BackEndUrlsStr string
)

// The function used to transform POST form to Item variable
// The return values are transformed Item variable and error
// The function also checks the formats of fields
func formToItem(form map[string][]string) (models.Item, error) {
	itemId := -1
	// If the id of the item is specified (update request), update the itemId
	if _, ok := form["itemId"]; ok {
		id, atoiErr := strconv.Atoi(form["itemId"][0])
		if atoiErr != nil {
			return models.Item{}, errors.New("Id is not valid!")
		}
		itemId = id
	}
	itemName := form["itemName"][0]
	itemDescription := form["itemDescription"][0]
	// Checks if value for unitPrice key is in float format
	// If not, return with error
	unitPrice, unitPriceErr := strconv.ParseFloat(form["unitPrice"][0], 32)
	if unitPriceErr != nil {
		return models.Item{}, errors.New("Unit Price is not a float!")
	}
	return models.Item{Id: itemId, ItemName: itemName, ItemDescription: itemDescription, UnitPrice: float32(unitPrice)}, nil
}

// A function to transform a map[int]models.Item to []models.Item
func mapToSlice(itemMap map[int]models.Item) []models.Item {
	var result []models.Item
	for _, value := range itemMap {
		result = append(result, value)
	}
	return result
}

func call(address string, calling string, args interface{}, reply interface{}) bool {
	c, dialErr := rpc.DialHTTP("tcp", address)
	if dialErr != nil {
		fmt.Println(dialErr.Error())
		return false
	}
	defer c.Close()

	callErr := c.Call(calling, args, reply)
	if callErr != nil {
		fmt.Println(callErr.Error())
		return false
	}
	return true
}

func requestBackEnds(args models.HandleClientRequestArguments, reply *models.HandleClientRequestReply) {
	for _, url := range BackEndUrls {
		success := call(url, "Server.HandleClientRequest", args, reply)
		if success {
			break
		}
	}
}

// The handler used to handle inventory request (/,/inventory)
func inventory(ctx iris.Context) {
	// Request for data
	var handleClientRequestArgs models.HandleClientRequestArguments
	var handleClientRequestReply models.HandleClientRequestReply
	handleClientRequestArgs.Method = "inventory"
	requestBackEnds(handleClientRequestArgs, &handleClientRequestReply)
	// Unmarshal reply
	var inventory models.Inventory
	unmarshalErr := json.Unmarshal(handleClientRequestReply.ResponseData, &inventory)
	itemSlice := mapToSlice(inventory.Items)
	if unmarshalErr != nil {
		fmt.Println(unmarshalErr.Error())
		return
	}
	// Present data
	ctx.View("inventory.html", itemSlice)
}

// The handler used to handle adding new Item request (/additem)
func addItem(ctx iris.Context) {
	form := ctx.FormValues()
	newItem, newItemErr := formToItem(form)
	// If transforming the form to Item variable failed, print the error,
	// wait for 3 seconds and redirect back to /additempage
	if newItemErr != nil {
		ctx.HTML(newItemErr.Error() + `<script type="text/javascript">
		setTimeout("location.href = '/additempage';",3000);
   		</script>`)
		return
	}
	// Marshal models.TCPRequest.RequestData
	newItemByte, marshalErr := json.Marshal(newItem)
	if marshalErr != nil {
		ctx.HTML(marshalErr.Error() + `<script type="text/javascript">
		setTimeout("location.href = '/additempage';",3000);
   		</script>`)
		return
	}
	// Request for data
	var handleClientRequestArgs models.HandleClientRequestArguments
	var handleClientRequestReply models.HandleClientRequestReply
	handleClientRequestArgs.Method = "add"
	handleClientRequestArgs.Variables = newItemByte
	requestBackEnds(handleClientRequestArgs, &handleClientRequestReply)
	ctx.Redirect("/inventory")
}

// The handler used to display update Item page (/updateitempage)
func updateItemPage(ctx iris.Context) {
	// Get original item info from url parameters and display
	params := ctx.URLParams()
	itemId, atoiErr := strconv.Atoi(params["itemId"])
	if atoiErr != nil {
		ctx.HTML(atoiErr.Error() + `<script type="text/javascript">
		setTimeout("location.href = '/inventory';",3000);
		</script>`)
		return
	}
	itemName := params["itemName"]
	itemDescription := params["itemDescription"]
	unitPrice, parseFloatErr := strconv.ParseFloat(params["unitPrice"], 32)
	if parseFloatErr != nil {
		ctx.HTML(parseFloatErr.Error() + `<script type="text/javascript">
		setTimeout("location.href = '/inventory';",3000);
		</script>`)
		return
	}
	var oldItem = models.Item{itemId, itemName, itemDescription, float32(unitPrice)}
	ctx.View("/updateItem.html", oldItem)
}

// The handler used to handle updating Item requests (/updateitem)
func updateItem(ctx iris.Context) {
	form := ctx.FormValues()
	newItem, newItemErr := formToItem(form)
	// If transforming the form to Item variable failed, print the error,
	// wait for 3 seconds and redirect back to /additempage
	if newItemErr != nil {
		ctx.HTML(newItemErr.Error() + `<script type="text/javascript">
		setTimeout("location.href = '/inventory';",3000);
   		</script>`)
		return
	}
	newItemByte, marshalErr := json.Marshal(newItem)
	if marshalErr != nil {
		ctx.HTML(marshalErr.Error() + `<script type="text/javascript">
		setTimeout("location.href = '/inventory';",3000);
   		</script>`)
		return
	}
	// Request for update
	var handleClientRequestArgs models.HandleClientRequestArguments
	var handleClientRequestReply models.HandleClientRequestReply
	handleClientRequestArgs.Method = "update"
	handleClientRequestArgs.Variables = newItemByte
	requestBackEnds(handleClientRequestArgs, &handleClientRequestReply)
	ctx.Redirect("/inventory")
}

// The handler used to handle delete Item requests
func deleteItem(ctx iris.Context) {
	form := ctx.FormValues()
	itemId, atoiErr := strconv.Atoi(form["itemId"][0])
	if atoiErr != nil {
		ctx.HTML(atoiErr.Error() + `<script type="text/javascript">
		setTimeout("location.href = '/inventory';",3000);
		</script>`)
		return
	}
	itemIdByte, marshalErr := json.Marshal(itemId)
	if marshalErr != nil {
		ctx.HTML(marshalErr.Error() + `<script type="text/javascript">
		setTimeout("location.href = '/inventory';",3000);
   		</script>`)
		return
	}
	// Request for delete
	var handleClientRequestArgs models.HandleClientRequestArguments
	var handleClientRequestReply models.HandleClientRequestReply
	handleClientRequestArgs.Method = "delete"
	handleClientRequestArgs.Variables = itemIdByte
	requestBackEnds(handleClientRequestArgs, &handleClientRequestReply)
	ctx.Redirect("/inventory")
}

// The function used to initialize an iris.Application variable and define routing
func initApplication() *iris.Application {
	app := iris.Default()
	// Load and register all .html templates in ./views
	tmpl := iris.HTML("./views", ".html")
	tmpl.Reload(true)
	app.RegisterView(tmpl)

	// Routing definition
	app.Handle("GET", "/", inventory)
	app.Handle("GET", "/inventory", inventory)
	app.Handle("GET", "/additempage", func(ctx iris.Context) { ctx.View("addItem.html") })
	app.Handle("POST", "/additem", addItem)
	app.Handle("GET", "/updateitempage", updateItemPage)
	app.Handle("POST", "/updateitem", updateItem)
	app.Handle("GET", "/deleteitem", deleteItem)
	return app
}

// Initialization before the program starts
func init() {
	// Retrieve the input, with 8080 as default value for listen, localhost:8090 for backend
	flag.IntVar(&FrontEndPort, "listen", 8080, "listen")
	flag.StringVar(&BackEndUrlsStr, "backend", ":8090,:8091,:8092", "backend")
	// Parse the command line inputs
	flag.Parse()
	// Fill out the full url BackEndUrl if only backend port is provided
	BackEndUrls = strings.Split(BackEndUrlsStr, ",")
	for i, url := range BackEndUrls {
		if strings.Index(url, ":") == 0 {
			BackEndUrls[i] = "localhost" + url
		}
	}
}

func main() {
	// Ensures the BackEndUrl in form hostname:port
	// columnIndex := strings.Index(BackEndUrl, ":")
	// _, err := strconv.Atoi(BackEndUrl[columnIndex+1:])
	// if err != nil {
	// 	fmt.Println("Error in backend input!")
	// 	return
	// }
	fmt.Printf("Backend urls are %s\n", BackEndUrls)
	// Initialize application
	app := initApplication()
	// Run the application
	app.Run(iris.Addr(fmt.Sprintf("localhost:%d", FrontEndPort)))
}
