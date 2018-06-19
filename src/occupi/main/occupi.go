package main

import (
  "os"
  "fmt"
  "flag"
  "time"
  "bytes"
  "strings"
  "net/http"
  "encoding/json"
  
  "github.com/stianeikeland/go-rpio"
)

var (
  DEBUG bool
  VERBOSE bool
  ENDPOINT string
)

type State int
const (
  LOCKED State = iota
  UNLOCKED
  UNKNOWN
)

type stateChange struct {
  State string  `json:"state"`
}

var httpClient = &http.Client{Timeout: 30 * time.Second}

func main() {

  cmdline   := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
  fRefresh  := cmdline.Duration ("refresh",   time.Millisecond * 50,              "GPIO pin refresh rate.")
  fEndpoint := cmdline.String   ("endpoint",  "",                                 "The endpoint to send lock state updates to. If omitted, updates are local only.")
  fDebug    := cmdline.Bool     ("debug",     strToBool(os.Getenv("DEBUG")),      "Enable debugging mode.")
  fVerbose  := cmdline.Bool     ("verbose",   strToBool(os.Getenv("VERBOSE")),    "Enable verbose debugging mode.")
  cmdline.Parse(os.Args[1:])

  fmt.Println(">>> Starting.")

  DEBUG = *fDebug
  VERBOSE = *fVerbose
  ENDPOINT = *fEndpoint

  refresh := *fRefresh
  if refresh < time.Millisecond {
    refresh = time.Millisecond
  }

  updateLockState(UNKNOWN)

  err := rpio.Open()
  if err != nil {
    panic(fmt.Errorf("Could not open GPIO: %v", err))
  }
  defer rpio.Close()

  indOpen := rpio.Pin(4)
  indOpen.Output()

  indLock := rpio.Pin(27)
  indLock.Output()

  indInput := rpio.Pin(17)
  indInput.Output()

  lock := rpio.Pin(9)
  lock.Input()

  button := rpio.Pin(11)
  button.Input()

  buzzer := rpio.Pin(18)
  buzzer.Output()

  indOpen.High()
  indLock.High()
  indInput.High()

  for i := 0; i < 20; i++ {
    if i % 3 == 0 {
      indOpen.Toggle()
    }else if i % 3 == 1 {
      indLock.Toggle()
    }else{
      indInput.Toggle()
    }
    <-time.After(time.Millisecond * 75)
  }

  indOpen.Low()
  indLock.Low()
  indInput.Low()

  go handleReset(button, indInput, indOpen, indLock, refresh)
  go handleLockUnlock(lock, indOpen, indLock, refresh)

  fmt.Println(">>> Ready.")

  <-make(chan struct{})
}

func handleReset(button, indInput, indOpen, indLock rpio.Pin, refresh time.Duration) {
  state := rpio.Low
  for {
    button.PullDown()
    res := button.Read()
    if state != res {
      state = res
      switch state {
      case rpio.High:
        if DEBUG || VERBOSE {
          fmt.Println(">>> BUTTON DOWN")
        }
        indOpen.Low()
        indLock.Low()
        indInput.Low()
      case rpio.Low:
        if DEBUG || VERBOSE {
          fmt.Println(">>> BUTTON UP")
        }
      }
    }
    <-time.After(refresh)
  }
}

func handleLockUnlock(lock, indOpen, indLock rpio.Pin, refresh time.Duration) {
  state := rpio.Low
  indOpen.High()
  for {
    lock.PullDown()
    res := lock.Read()
    if state != res {
      state = res

      switch state {
      case rpio.High:
        if DEBUG || VERBOSE {
          fmt.Println(">>> LOCK DOWN")
        }
        indOpen.Low()
        indLock.High()
      case rpio.Low:
        if DEBUG || VERBOSE {
          fmt.Println(">>> LOCK UP")
        }
        indOpen.High()
        indLock.Low()
      }

      var update State
      switch state {
      case rpio.High:
        update = LOCKED
      case rpio.Low:
        update = UNLOCKED
      default:
        update = UNKNOWN
      }

      err := updateLockState(update)
      if err != nil {
        fmt.Errorf(">>> Could not update lock state: %v", err)
      }
    }
    <-time.After(refresh)
  }
}

func updateLockState(s State) error {
  if VERBOSE || DEBUG {
    fmt.Printf(">>> Updating state: %v\n", s)
  }

  var dest string
  switch s {
    case LOCKED:
      dest = "occupied"
    case UNLOCKED:
      dest = "available"
    default:
      dest = "unknown"
  }

  change := &stateChange{
    State: dest,
  }
  data, err := json.Marshal(change)
  if err != nil {
    return fmt.Errorf("Could not marshal state change: %v", err)
  }

  req, err := http.NewRequest("POST", ENDPOINT, bytes.NewBuffer(data))
  if err != nil {
    return fmt.Errorf("Could not create update request: %v", err)
  }

  rsp, err := httpClient.Do(req)
  if err != nil {
    return fmt.Errorf("Could not perform update request: %v", err)
  }

  if rsp.StatusCode != http.StatusOK {
    return fmt.Errorf("Invalid status code from service: %v", rsp.Status)
  }

  return nil
}

func strToBool(s string) bool {
  return strings.EqualFold(s, "t") || strings.EqualFold(s, "true") || strings.EqualFold(s, "y") || strings.EqualFold(s, "yes")
}