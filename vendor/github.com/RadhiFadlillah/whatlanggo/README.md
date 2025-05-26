# Whatlanggo [![Go Reference][goref-badge]][goref-page]

Natural language detection for Go. Forked from the [original][original-repo] project by [Abado Jack Mtulla][abadojack], which is a derivative of [Franc][franc] (JavaScript, MIT) by [Titus Wormer][franc-author].

## Table of Contents

- [Features](#features)
- [Installation](#installation)
- [Usage](#usage)
  - [Basic Usage](#basic-usage)
  - [Blacklisting and whitelisting](#blacklisting-and-whitelisting)
- [How does it work?](#how-does-it-work)
  - [How does the language recognition work?](#how-does-the-language-recognition-work)
  - [How _IsReliable_ calculated?](#how-isreliable-calculated)
- [License](#license)
- [Acknowledgements](#acknowledgements)

## Features

- Supports [84 languages](./SUPPORTED_LANGUAGES.md).
- 100% written in Go.
- Fast.
- No external dependencies.
- Recognizes not only a language, but also a script (Latin, Cyrillic, etc).

## Installation

Installation:

```sh
go get -u github.com/RadhiFadlillah/whatlanggo
```

## Usage

### Basic Usage

Simple usage example:

```go
package main

import (
	"fmt"

	"github.com/RadhiFadlillah/whatlanggo"
)

func main() {
	info := whatlanggo.Detect("Foje funkcias kaj foje ne funkcias")
	fmt.Println("Language:", info.Lang.String(), " Script:", whatlanggo.Scripts[info.Script], " Confidence: ", info.Confidence)
}
```

### Blacklisting and whitelisting

```go
package main

import (
	"fmt"

	"github.com/RadhiFadlillah/whatlanggo"
)

func main() {
	//Blacklist
	options := whatlanggo.Options{
		Blacklist: map[whatlanggo.Lang]bool{
			whatlanggo.Ydd: true,
		},
	}

	info := whatlanggo.DetectWithOptions("האקדמיה ללשון העברית", options)

	fmt.Println("Language:", info.Lang.String(), "Script:", whatlanggo.Scripts[info.Script])

	//Whitelist
	options1 := whatlanggo.Options{
		Whitelist: map[whatlanggo.Lang]bool{
			whatlanggo.Epo: true,
			whatlanggo.Ukr: true,
		},
	}

	info = whatlanggo.DetectWithOptions("Mi ne scias", options1)
	fmt.Println("Language:", info.Lang.String(), " Script:", whatlanggo.Scripts[info.Script])
}
```

For more details, please check the [documentation][goref-page].

## How does it work?

### How does the language recognition work?

The algorithm is based on the trigram language models, which is a particular case of n-grams. To understand the idea, please check the original whitepaper [Cavnar and Trenkle '94: N-Gram-Based Text Categorization'][original-paper].

### How _IsReliable_ calculated?

It is based on the following factors:

- How many unique trigrams are in the given text
- How big is the difference between the first and the second(not returned) detected languages? This metric is called `rate` in the code base.

Therefore, it can be presented as 2d space with threshold functions, that splits it into "Reliable" and "Not reliable" areas.
This function is a hyperbola and it looks like the following one:

<img alt="Language recognition whatlang rust" src="https://raw.githubusercontent.com/RadhiFadlillah/whatlanggo/master/images/whatlang_is_reliable.png" width="450" height="300" />

For more details, please check a blog article [Introduction to Rust Whatlang Library and Natural Language Identification Algorithms][greyblake-blog].

## License

Like the original project, this fork is licensed under [MIT License](./LICENSE)

## Acknowledgements

Thanks to [greyblake] (Potapov Sergey) for creating [whatlang-rs] from where I got the idea and algorithms.

[goref-badge]: https://pkg.go.dev/badge/github.com/RadhiFadlillah/whatlanggo.svg
[goref-page]: https://pkg.go.dev/github.com/RadhiFadlillah/whatlanggo
[original-repo]: https://github.com/abadojack/whatlanggo
[abadojack]: https://github.com/abadojack
[franc]: https://github.com/wooorm/franc
[franc-author]: https://github.com/wooorm
[original-paper]: https://www.researchgate.net/publication/2375544_N-Gram-Based_Text_Categorization
[greyblake]: https://github.com/greyblake
[greyblake-blog]: https://www.greyblake.com/blog/2017-07-30-introduction-to-rust-whatlang-library-and-natural-language-identification-algorithms/
[whatlang-rs]: https://github.com/greyblake/whatlang-rs
