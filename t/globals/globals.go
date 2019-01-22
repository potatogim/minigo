package main

import "fmt"

func eval() {
	fmt.Printf("%d\n", gint)
	fmt.Printf("%c\n", gchar)
	if gtrue {
		fmt.Printf("3\n")
	}
	if !gfalse {
		fmt.Printf("4\n")
	}

	fmt.Printf("%d\n", gstruct.gint)  // 5
	fmt.Printf("%c\n", gstruct.gchar) // 6
	if gstruct.gtrue {
		fmt.Printf("7\n")
	}
	if !gstruct.gfalse {
		fmt.Printf("8\n")
	}

	fmt.Printf("%d\n", garray[0]) // 9
	fmt.Printf("%d\n", garray[1]) // 10
	fmt.Printf("%d\n", garray[2]) // 11

}

func evalnested() {
	// nested data
	fmt.Printf("%d\n", gstruct.inner.gint) // 12
	fmt.Printf("%d\n", gstruct.inner.inner.gchar) // 13
	if gstruct.inner.inner.gtrue == true {
		fmt.Printf("14\n")
	}
}

/*
func evalnestedarary() {
	var i int = gpoints[2].Y // TBI
	fmt.Printf("%d\n", i + 9) //15
}
*/

func assign() {

}

func main() {
	eval()
	evalnested()
	// evalnestedarary() // TBI
	assign()
}


var gint int = 1
var gchar byte = '2'
var gtrue bool = true
var gfalse bool = false

var gstruct = MyStruct{
	gint:5,
	gchar:'6',
	gtrue:true,
	gfalse:false,
	inner: MyInnerStruct{
		gint:12,
		gtrue: true,
		inner: MyInnerInnerStruct{
			gtrue:true,
			gchar:13,
		},
	},
}

var garray [3]int = [...]int{9,10,11}
var gpoints [3]Point = [...]Point{
	Point{
		X:1,
		Y:2,
	},
	Point{
		X:3,
		Y:4,
	},
	Point{
		X:5,
		Y:6,
	},
}

type MyStruct struct {
	gint int
	gchar byte
	gtrue bool
	gfalse bool
	inner MyInnerStruct
}

type MyInnerStruct struct {
	gint int
	gchar byte
	gtrue bool
	gfalse bool
	inner MyInnerInnerStruct
}

type MyInnerInnerStruct struct {
	gint int
	gchar byte
	gtrue bool
	gfalse bool
}

type Point struct {
	X int
	Y int
}