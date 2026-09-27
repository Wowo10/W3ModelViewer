package viewer

// cimgui-go's code generator skips ImGui_ImplSDL2_ProcessEvent (it takes the
// SDL_Event union), so we declare it ourselves. The symbol comes from the
// cimgui archive that impl/sdl2 already links in.

/*
#include <stdbool.h>

extern bool ImGui_ImplSDL2_ProcessEvent(const void *event);
*/
import "C"

import (
	"reflect"
	"unsafe"

	"github.com/veandco/go-sdl2/sdl"
)

// processEvent hands an SDL event to the ImGui backend so it can track mouse,
// keyboard and focus state.
//
// go-sdl2's typed events are views onto the C SDL_Event buffer filled by
// SDL_PollEvent, so the address of the concrete struct is the address ImGui
// expects. sdl.DropEvent is the one exception - it is a Go-side copy - but its
// leading type field still reads correctly and ImGui ignores drop events.
func processEvent(event sdl.Event) {
	if event == nil {
		return
	}

	v := reflect.ValueOf(event)
	if v.Kind() != reflect.Ptr || v.IsNil() {
		return
	}

	C.ImGui_ImplSDL2_ProcessEvent(unsafe.Pointer(v.Pointer()))
}
