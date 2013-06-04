package main

import (
    "github.com/mbsulliv/termbox-go"
    "math/rand"
    "runtime/pprof"
    "os"
    "log"
)

var bufs [][]termbox.Cell

func draw(aWidth int, aBufNum int) {
    termbox.Blit(0,0, aWidth, bufs[aBufNum])
	termbox.Flush()
}

func main() {
    vF,vProfErr := os.Create("dat.prof")
    if vProfErr != nil { log.Fatal(vProfErr) }
    pprof.StartCPUProfile(vF)
    defer pprof.StopCPUProfile()

	vErr := termbox.Init()
	if vErr != nil { panic(vErr) }
	defer termbox.Close()

	vEventQueue := make(chan termbox.Event)
	go func() {
		for {
            var vEv termbox.Event
            termbox.PollEvent(&vEv)
			vEventQueue <- vEv
		}
	}()

    vQuitCnt := 200
    vMaxBuf  := 10
    vCurBuf  := vMaxBuf-1

	vW,vH := termbox.Size()
    bufs = make([][]termbox.Cell, vMaxBuf)
    for vI := 0 ; vI < vMaxBuf ; vI++ {
        bufs[vI] = make([]termbox.Cell, vW*vH)
        for vY := 0 ; vY < vH ; vY++ {
            for vX := 0 ; vX < vW ; vX++ {
                bufs[vI][vY*vW+vX] = termbox.Cell{' ', termbox.ColorDefault, termbox.Attribute(rand.Int() % 8)+1}
            }
        }
    }

	draw(vW, vCurBuf)
    vCurBuf--
    loop: for {
		select {
		case vEv := <-vEventQueue:
			if vEv.Type == termbox.EventKey && vEv.Key == termbox.KeyEsc { break loop }
		default:
			draw(vW, vCurBuf)
            vCurBuf--
            if vCurBuf == 0 { vCurBuf = vMaxBuf-1 }
            vQuitCnt--
            if vQuitCnt == 0 { break loop }
		}
	}
}
