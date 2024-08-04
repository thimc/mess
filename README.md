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
are there, I have also added some for convenience.

`$` to go to the last message

`0` to go to the first message

`D` prompts the current message for deletion

`G` / `Home` to scroll to the top

`H` toggles forces the mail to be rendered in HTML mode

`J` goes to next mail

`K` goes to previous mail

`N` to go to the next unseen message

`R` toggles the raw mode which prints the files content without any rendering.

`^` goto the parent mail

`c` opens the `$EDITOR` and lets the user compose a new mail

`d` marks the current message as read

`f` opens the `$EDITOR` and runs mfwd, to forward a mail

`g` / `End` to scroll the current mail to the bottom

`j` / `Arrow down` / `Enter` scrolls the current mail one line down

`k` / `Arrow up` scrolls the current mail one line up

`r` opens the `$EDITOR` and runs mrep, to reply to a mail

`u` marks the current message as unread

`q` to quit

`Ctrl+d` / `Page Down` to scroll the mail one page down

`Ctrl+u` / `Page Up` to scroll the mail one page up

## License

MIT

