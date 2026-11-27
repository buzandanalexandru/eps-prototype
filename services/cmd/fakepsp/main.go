// Command fakepsp is the simulated payment provider (O4). No real payments, ever. Step 0 serves
// health and readiness only; the provider endpoints arrive in step 4.
package main

import (
	"github.com/buzandanalexandru/eps-prototype/services/internal/service"
)

func main() {
	log := service.Logger("fakepsp")
	service.Serve(log, service.SimpleHandler("fakepsp", log, nil, nil, nil))
}
