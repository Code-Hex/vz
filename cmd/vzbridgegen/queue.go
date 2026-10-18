package main

func inheritsMachineQueue(owner, selector string) bool {
	switch owner + "." + selector {
	case "VZVirtualMachine.consoleDevices",
		"VZVirtualMachine.directorySharingDevices",
		"VZVirtualMachine.graphicsDevices",
		"VZVirtualMachine.memoryBalloonDevices",
		"VZVirtualMachine.networkDevices",
		"VZVirtualMachine.socketDevices",
		"VZVirtualMachine._storageDevices",
		"VZVirtualMachine.usbControllers",
		"VZGraphicsDevice.displays",
		"VZVirtioConsoleDevice.ports",
		"VZVirtioConsolePortArray.objectAtIndexedSubscript:",
		"VZUSBController.usbDevices",
		"VZMacOSInstaller.initWithVirtualMachine:restoreImageURL:",
		"VZCustomVirtioDevice.guestMemoryMappingAtPhysicalAddress:length:",
		"VZCustomVirtioDevice.queueAtIndex:",
		"VZCustomVirtioDevice.sharedMemoryRegions",
		"VZVirtioQueue.nextElement":
		return true
	default:
		return false
	}
}
