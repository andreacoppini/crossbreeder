# Crossbreeder #

Bulk firmware changing and basic action tool for standalone Ruckus Access Points.
----------
> Tool    : Crossbreeder<br>
> Author  : Andrea Coppini<br>
> Feedback: Email andrea@tacoppini.com with your feedback.<br>
----------
## Description ##

Crossbreeder is a troubleshooting and automating some simple 
commonly used tasks for Ruckus APs such as factory reset, update firmware etc.
It does not rely on any controller.  Instead, it runs through a list of IP
addresses supplied by the user to contact each AP directly via SSH.
This utility is built for windows and macOS platforms.

## Where do I get it? ##

Get it from here!

## How do I install? ##

There’s no installer, just unzip and run the exe.

## How do I use it? ##

It should be fairly self-explanatory:


1. you feed it a CSV file
1. you set the AP username/password and/or check the ‘also try default’ to use super/sp-admin on factory default APs. (It will try whatever you set first, for example “admin”/”Ruckus123”, and if that fails it will try “super”/”sp-admin”)
1. you choose what you want to do with the APs.  It can be any or none of the below:
	1. Change Firmware; to change the AP firmware. (You need to supply your own HTTP, FTP or TFTP server)
	1. Reset AP to factory defaults; same as typing "set factory"
	1. Run a custom command; run any AP CLI command such as ‘set scg ip x.x.x.x’
	1. Reboot the AP; same as typing "reload"
1. If you do not check any of the above 4 options, it will just collect information about the APs and display them in the table.
1. If you wish, you can save the results to a JSON or CSV file.

----------
