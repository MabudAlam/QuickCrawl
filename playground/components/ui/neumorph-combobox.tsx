"use client"

import { CheckIcon, ChevronsUpDown } from "lucide-react"
import * as React from "react"

import { Button } from "@/components/ui/button"
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from "@/components/ui/command"
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover"

import { cn } from "@/lib/utils"

interface ComboboxOption {
  value: string
  label: string
}

interface NeumorphComboboxProps {
  value: string
  onValueChange: (value: string) => void
  options: ComboboxOption[]
  placeholder?: string
  className?: string
  disabled?: boolean
}

export function NeumorphCombobox({
  value,
  onValueChange,
  options,
  placeholder = "Select...",
  className,
  disabled,
}: NeumorphComboboxProps) {
  const [open, setOpen] = React.useState(false)
  const [search, setSearch] = React.useState("")

  const filteredOptions = options.filter((option) =>
    option.label.toLowerCase().includes(search.toLowerCase())
  )

  const selectedLabel = options.find((opt) => opt.value === value)?.label

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          variant="noShadow"
          role="combobox"
          aria-expanded={open}
          disabled={disabled}
          className={cn(
            "w-full justify-between px-4 py-2.5 rounded-base text-sm font-medium transition-all duration-200",
            "bg-background border-2 border-border",
            "shadow-shadow hover:translate-x-boxShadowX hover:translate-y-boxShadowY hover:shadow-none",
            className
          )}
        >
          <span className={cn("truncate", !selectedLabel && "text-muted-foreground")}>
            {selectedLabel || placeholder}
          </span>
          <ChevronsUpDown className="ml-2 h-4 w-4 shrink-0 opacity-50" />
        </Button>
      </PopoverTrigger>
      <PopoverContent
        className="w-full p-0"
        style={{ width: "var(--radix-popover-trigger-width)" }}
      >
        <Command className="rounded-base overflow-hidden bg-background border-2 border-border shadow-shadow">
          <CommandInput
            placeholder="Search..."
            value={search}
            onValueChange={setSearch}
            className="h-11 border-0 bg-transparent"
          />
          <CommandList className="p-1 max-h-[200px]">
            <CommandEmpty className="py-6 text-center text-sm text-muted-foreground">
              No option found.
            </CommandEmpty>
            <CommandGroup>
              {filteredOptions.map((option) => (
                <CommandItem
                  key={option.value}
                  value={option.value}
                  onSelect={(currentValue) => {
                    onValueChange(currentValue === value ? "" : currentValue)
                    setOpen(false)
                    setSearch("")
                  }}
                  className="flex items-center justify-between px-3 py-2 cursor-pointer hover:bg-secondary-background rounded-lg mx-1"
                >
                  <span className="truncate">{option.label}</span>
                  <CheckIcon
                    className={cn(
                      "h-4 w-4 shrink-0",
                      value === option.value ? "opacity-100" : "opacity-0"
                    )}
                  />
                </CommandItem>
              ))}
            </CommandGroup>
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  )
}