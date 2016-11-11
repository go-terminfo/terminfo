# terminfo

Need to address a terminal's features in a <a name=portable>portable[¹](#fportable)</a>
way? Don't want to make your executable dynamic, or deal with CGO weirdness? Then
this might be what you're looking for.

This is a pure-go implementation of a terminfo database
<a name=reader>reader[²](#fdatabase)</a>, plus some functions to transform those into
things you can blat to your terminal for it to actually do what you want
(otherwise you'd get things like `"\x1b[%i%p1%d;%p2%dH$<5>"` which are
very entertaining but not particularly useful).

There are probably bugs in this, as it hasn't yet been used in anger.  Drop me a note.

There are some differences with how it interprets the terminfo data
compared to <a name=diff>ncurses[³](#fdiff)</a>. Those are probably bugs too.

## Notes

1. <a name=fportable></a>
   I've tested it by hand on as many terminals as I have access to,
   but that's not a lot of weird ones. Also I've only tested it on
   Linux. I hear you chant “portability, schmortability!”; I'll gladly
   take patches (or even suggestions for me to implement) aimed at
   making it actually cross-platform. [🔙](#portable)

1. <a name=fdatabase></a>
   It only supports compiled databases in the “directory tree” style,
   not hashed databases, for now. Patches welcome! [🔙](#database)

1. <a name=fdiff></a>
   For example, my reading of how to do pads means you actually get a
   flash from `FlashScreen` (vs. `tput flash`) on `xterm`, but also
   means the `linux` terminal terminfo's `flash` entry of
   `"\x1b[?5h\x1b[?5l$<200/>"`
   (i.e. only pad *after* it switches back to normal) doesn't make
   sense nor make the flash as visible as you'd presumably want.
   [🔙](#diff)
