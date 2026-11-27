// Command simulator drives virtual fans and bots against a rehearsal sale (step 6). It runs as a
// Kubernetes Job, disabled by default. Step 0 serves health and readiness only.
package main

import (
	"github.com/buzandanalexandru/eps-prototype/services/internal/service"
)

func main() {
	log := service.Logger("simulator")
	service.Serve(log, service.SimpleHandler("simulator", log, nil, nil, nil))
}
