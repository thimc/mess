# mess

Mess is a TUI for [mblaze](https://github.com/leahneukirchen/mblaze)
and was created as a result of OpenBSD dropping support for `$LESSOPEN`
in their [less](https://man.openbsd.org/less.1) implementation
([mail thread](https://marc.info/?l=openbsd-tech&m=171310714302503&w=2))
and thus, making it impossible to use mless unless you have an older
version of the less binary backed up on your system.

*Note: mess makes heavy use of the mblaze programs so it goes without
saying that you need to have mblaze installed to use mess.*

**NOTE: While mess works as expected (most of the time) it's still
in its early days, there are still a lot of things to clean up and
get right. Be warned**

## Keybindings

Most of the key binds you would expect from
[mless](https://github.com/leahneukirchen/mblaze/blob/master/man/mless.1)
are there, I have also added some for convenience. Here is a
non-exhaustive list of some of them:

q to quit

`j` / `Arrow down` / `Enter` scrolls the current mail one line down

`k` / `Arrow up` scrolls the current mail one line up

`J` goes to next mail

`K` goes to previous mail

`0` to go to the first message

`$` to go to the last message

`N` to go to the next unseen message

`D` prompts the current message for deletion

`c` opens the `$EDITOR` and lets the user compose a new mail

`f` opens the `$EDITOR` and runs mfwd, to forward a mail

`r` opens the `$EDITOR` and runs mrep, to reply to a mail

`d` marks the current message as read

`u` marks the current message as unread

`R` toggles the raw mode which prints the files content without any rendering.

`H` toggles forces the mail to be rendered in HTML mode

`^` goto the parent mail

## License

MIT

