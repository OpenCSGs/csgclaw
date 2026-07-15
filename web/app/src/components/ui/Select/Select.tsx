import { Select as RadixSelect } from "radix-ui";
import { Check, ChevronDown, ChevronUp, Search } from "lucide-react";
import { forwardRef, useLayoutEffect, useMemo, useRef, useState } from "react";
import type { ComponentPropsWithoutRef, ComponentRef, ReactNode } from "react";
import { classNames } from "@/shared/lib/classNames";

export type SelectSize = "sm" | "md";

export type SelectOption = {
  description?: ReactNode;
  disabled?: boolean;
  label: ReactNode;
  textValue?: string;
  value: string;
};

const triggerSizeClassNames: Record<SelectSize, string> = {
  sm: "csg-select-trigger-sm",
  md: "csg-select-trigger-md",
};

const SELECT_EMPTY_ITEM_VALUE = "__csg-select-empty-value__";

function toRadixValue(value: string | undefined, hasEmptyOption: boolean) {
  return hasEmptyOption && value === "" ? SELECT_EMPTY_ITEM_VALUE : value;
}

function fromRadixValue(value: string) {
  return value === SELECT_EMPTY_ITEM_VALUE ? "" : value;
}

export type SelectRootProps = ComponentPropsWithoutRef<typeof RadixSelect.Root>;

export function SelectRoot(props: SelectRootProps) {
  return <RadixSelect.Root {...props} />;
}

export type SelectGroupProps = ComponentPropsWithoutRef<typeof RadixSelect.Group>;

export const SelectGroup = forwardRef<ComponentRef<typeof RadixSelect.Group>, SelectGroupProps>(
  function SelectGroup(props, ref) {
    return <RadixSelect.Group ref={ref} {...props} />;
  },
);

export type SelectValueProps = ComponentPropsWithoutRef<typeof RadixSelect.Value>;

export const SelectValue = forwardRef<ComponentRef<typeof RadixSelect.Value>, SelectValueProps>(
  function SelectValue(props, ref) {
    return <RadixSelect.Value ref={ref} {...props} />;
  },
);

export type SelectTriggerProps = ComponentPropsWithoutRef<typeof RadixSelect.Trigger> & {
  placeholder?: SelectValueProps["placeholder"];
  size?: SelectSize;
};

export const SelectTrigger = forwardRef<ComponentRef<typeof RadixSelect.Trigger>, SelectTriggerProps>(
  function SelectTrigger({ children, className, placeholder, size = "md", ...props }, ref) {
    return (
      <RadixSelect.Trigger
        ref={ref}
        className={classNames(
          "csg-select-trigger inline-flex w-full items-center justify-between gap-2",
          triggerSizeClassNames[size],
          className,
        )}
        {...props}
      >
        <span className="csg-select-value min-w-0 flex-1 truncate">
          {children ?? <RadixSelect.Value placeholder={placeholder} />}
        </span>
        <RadixSelect.Icon asChild>
          <ChevronDown className="csg-select-chevron shrink-0" aria-hidden="true" size={16} strokeWidth={2} />
        </RadixSelect.Icon>
      </RadixSelect.Trigger>
    );
  },
);

export type SelectContentProps = ComponentPropsWithoutRef<typeof RadixSelect.Content> & {
  portalContainer?: ComponentPropsWithoutRef<typeof RadixSelect.Portal>["container"];
};

export const SelectContent = forwardRef<ComponentRef<typeof RadixSelect.Content>, SelectContentProps>(
  function SelectContent({ children, className, portalContainer, position = "popper", sideOffset = 6, ...props }, ref) {
    return (
      <RadixSelect.Portal container={portalContainer}>
        <RadixSelect.Content
          ref={ref}
          className={classNames("csg-select-content overflow-hidden", className)}
          position={position}
          sideOffset={sideOffset}
          {...props}
        >
          <RadixSelect.ScrollUpButton className="csg-select-scroll-button flex items-center justify-center">
            <ChevronUp aria-hidden="true" size={16} strokeWidth={2} />
          </RadixSelect.ScrollUpButton>
          <RadixSelect.Viewport className="csg-select-viewport p-1">{children}</RadixSelect.Viewport>
          <RadixSelect.ScrollDownButton className="csg-select-scroll-button flex items-center justify-center">
            <ChevronDown aria-hidden="true" size={16} strokeWidth={2} />
          </RadixSelect.ScrollDownButton>
        </RadixSelect.Content>
      </RadixSelect.Portal>
    );
  },
);

export type SelectLabelProps = ComponentPropsWithoutRef<typeof RadixSelect.Label>;

export const SelectLabel = forwardRef<ComponentRef<typeof RadixSelect.Label>, SelectLabelProps>(function SelectLabel(
  { className, ...props },
  ref,
) {
  return <RadixSelect.Label ref={ref} className={classNames("csg-select-label px-2 py-1.5", className)} {...props} />;
});

export type SelectItemProps = ComponentPropsWithoutRef<typeof RadixSelect.Item> & {
  description?: ReactNode;
};

export const SelectItem = forwardRef<ComponentRef<typeof RadixSelect.Item>, SelectItemProps>(function SelectItem(
  { children, className, description, ...props },
  ref,
) {
  return (
    <RadixSelect.Item
      ref={ref}
      className={classNames(
        "csg-select-item relative flex w-full items-center rounded-md py-2 pr-3 pl-8",
        description ? "csg-select-item-with-description" : "",
        className,
      )}
      {...props}
    >
      <span className="csg-select-item-indicator absolute left-2 inline-flex items-center justify-center">
        <RadixSelect.ItemIndicator>
          <Check aria-hidden="true" size={16} strokeWidth={2} />
        </RadixSelect.ItemIndicator>
      </span>
      <span className="csg-select-item-copy">
        <RadixSelect.ItemText>{children}</RadixSelect.ItemText>
        {description ? <span className="csg-select-item-description">{description}</span> : null}
      </span>
    </RadixSelect.Item>
  );
});

export type SelectSeparatorProps = ComponentPropsWithoutRef<typeof RadixSelect.Separator>;

export const SelectSeparator = forwardRef<ComponentRef<typeof RadixSelect.Separator>, SelectSeparatorProps>(
  function SelectSeparator({ className, ...props }, ref) {
    return (
      <RadixSelect.Separator
        ref={ref}
        className={classNames("csg-select-separator -mx-1 my-1", className)}
        {...props}
      />
    );
  },
);

export type SelectProps = Omit<SelectRootProps, "children"> & {
  children?: ReactNode;
  contentClassName?: string;
  contentProps?: Omit<SelectContentProps, "children" | "className">;
  emptyLabel?: ReactNode;
  options?: readonly SelectOption[];
  placeholder?: ReactNode;
  searchable?: boolean;
  searchPlaceholder?: string;
  selectedLabel?: ReactNode;
  size?: SelectSize;
  triggerClassName?: string;
  triggerProps?: Omit<SelectTriggerProps, "children" | "className" | "placeholder" | "size">;
};

function optionSearchText(option: SelectOption) {
  if (option.textValue) {
    return option.textValue;
  }
  if (typeof option.label === "string" || typeof option.label === "number") {
    return String(option.label);
  }
  return option.value;
}

export function Select({
  children,
  contentClassName,
  contentProps,
  defaultValue,
  emptyLabel,
  onValueChange,
  onOpenChange,
  options,
  placeholder,
  searchable = false,
  searchPlaceholder = "Search",
  selectedLabel,
  size = "md",
  triggerClassName,
  triggerProps,
  value,
  ...props
}: SelectProps) {
  const [searchQuery, setSearchQuery] = useState("");
  const contentRef = useRef<ComponentRef<typeof RadixSelect.Content>>(null);
  const searchInputRef = useRef<HTMLInputElement>(null);
  const hasEmptyOption = Boolean(options?.some((option) => option.value === ""));
  const normalizedSearchQuery = searchQuery.trim().toLocaleLowerCase();
  const filteredOptions = useMemo(() => {
    if (!options) {
      return [];
    }
    if (!searchable || !normalizedSearchQuery) {
      return options;
    }
    return options.filter((option) => {
      if (option.value === "") {
        return false;
      }
      return optionSearchText(option).toLocaleLowerCase().includes(normalizedSearchQuery);
    });
  }, [normalizedSearchQuery, options, searchable]);
  const shouldRenderSearch = searchable && Boolean(options?.length);
  const resolvedContentProps = searchable
    ? ({
        side: "bottom",
        align: "start",
        avoidCollisions: false,
        ...contentProps,
      } satisfies SelectProps["contentProps"])
    : contentProps;

  useLayoutEffect(() => {
    if (!shouldRenderSearch || typeof document === "undefined") {
      return;
    }
    const activeElement = document.activeElement;
    if (
      searchInputRef.current &&
      contentRef.current?.contains(activeElement) &&
      activeElement !== searchInputRef.current
    ) {
      searchInputRef.current.focus({ preventScroll: true });
    }
  }, [searchQuery, shouldRenderSearch, filteredOptions.length]);

  return (
    <SelectRoot
      value={toRadixValue(value, hasEmptyOption)}
      defaultValue={toRadixValue(defaultValue, hasEmptyOption)}
      onValueChange={onValueChange ? (nextValue) => onValueChange(fromRadixValue(nextValue)) : undefined}
      onOpenChange={(open) => {
        if (!open) {
          setSearchQuery("");
        }
        onOpenChange?.(open);
      }}
      {...props}
    >
      <SelectTrigger className={triggerClassName} placeholder={placeholder} size={size} {...triggerProps}>
        {selectedLabel}
      </SelectTrigger>
      <SelectContent ref={contentRef} className={contentClassName} {...resolvedContentProps}>
        {children ??
          (options ? (
            <>
              {shouldRenderSearch ? (
                <div className="csg-select-search" onPointerDown={(event) => event.stopPropagation()}>
                  <Search aria-hidden="true" size={15} strokeWidth={2} />
                  <input
                    ref={searchInputRef}
                    aria-label={searchPlaceholder}
                    autoComplete="off"
                    onBlur={(event) => {
                      const nextFocusedElement = event.relatedTarget;
                      if (
                        contentRef.current &&
                        nextFocusedElement instanceof Node &&
                        contentRef.current.contains(nextFocusedElement)
                      ) {
                        const input = event.currentTarget;
                        queueMicrotask(() => {
                          if (
                            contentRef.current?.contains(document.activeElement) &&
                            document.activeElement !== input
                          ) {
                            input.focus({ preventScroll: true });
                          }
                        });
                      }
                    }}
                    onChange={(event) => setSearchQuery(event.currentTarget.value)}
                    onKeyDownCapture={(event) => {
                      if (event.key !== "Escape" && event.key !== "Tab") {
                        event.stopPropagation();
                      }
                    }}
                    onKeyDown={(event) => {
                      if (event.key !== "Escape" && event.key !== "Tab") {
                        event.stopPropagation();
                      }
                    }}
                    placeholder={searchPlaceholder}
                    type="search"
                    value={searchQuery}
                  />
                </div>
              ) : null}
              {filteredOptions.length ? (
                filteredOptions.map((option) => (
                  <SelectItem
                    key={option.value}
                    value={toRadixValue(option.value, hasEmptyOption) ?? option.value}
                    disabled={option.disabled}
                    description={option.description}
                    textValue={option.textValue}
                  >
                    {option.label}
                  </SelectItem>
                ))
              ) : (
                <div className="csg-select-empty">{emptyLabel ?? "No options"}</div>
              )}
            </>
          ) : null)}
      </SelectContent>
    </SelectRoot>
  );
}
