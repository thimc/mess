# mess

Mess is a TUI for [mblaze](https://github.com/leahneukirchen/mblaze)
and was created as a result of OpenBSD dropping support for `$LESSOPEN`
in their [less](https://man.openbsd.org/less.1) implementation
([mail thread](https://marc.info/?l=openbsd-tech&m=171310714302503&w=2))
and thus, making it impossible to use mless unless you have an older
version of the less binary backed up on your system.

*Note: mess makes heavy use of the mblaze programs so it goes without
saying that you need to have mblaze installed to use mess.*

## Keybindings

Mess uses `$PAGER` to display the current mail, some keybindings
(the list below) are handled by mess, any other key is passed to
the program drawing the mail itself.

`^` go to the parent mail in a mail thread.

`0` go to the first message.

`$` go to the last message.

`c` compose a new mail using mcom (which utilizes `$EDITOR`).

`d` mark the current mail as read.

`f` forward the current mail using mfwd.

`q` to quit.

`r` reply the current mail using mrep.

`u` mark the current mail as unread.

`D` / `Delete` delete the current message (the user will be given
a prompt before any changes are actually made on disk).

`H` force render the mail as a `text/html` mail.

`J` go to the next mail.

`K` go to the previous mail.

`N` go to the next unseen mail.

`R` print the raw file contents of the mail instead of rendering it via mshow.

`T` go to the next mail thread.

## License

MIT

