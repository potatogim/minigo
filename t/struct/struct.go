package main

import "fmt"


func main() {
	var i int = 1
	var u user = user {
		id: 3,
		age: 2,
	}
	fmt.Printf("%d\n", i)
	fmt.Printf("%d\n", u.age)
	fmt.Printf("%d\n", u.id)
}

type user struct {
	id int
	age int
}
