var __defProp = Object.defineProperty;
var __defProps = Object.defineProperties;
var __getOwnPropDescs = Object.getOwnPropertyDescriptors;
var __getOwnPropSymbols = Object.getOwnPropertySymbols;
var __hasOwnProp = Object.prototype.hasOwnProperty;
var __propIsEnum = Object.prototype.propertyIsEnumerable;
var __defNormalProp = (obj, key, value) => key in obj ? __defProp(obj, key, { enumerable: true, configurable: true, writable: true, value }) : obj[key] = value;
var __spreadValues = (a, b) => {
  for (var prop in b || (b = {}))
    if (__hasOwnProp.call(b, prop))
      __defNormalProp(a, prop, b[prop]);
  if (__getOwnPropSymbols)
    for (var prop of __getOwnPropSymbols(b)) {
      if (__propIsEnum.call(b, prop))
        __defNormalProp(a, prop, b[prop]);
    }
  return a;
};
var __spreadProps = (a, b) => __defProps(a, __getOwnPropDescs(b));
var __async = (__this, __arguments, generator) => {
  return new Promise((resolve, reject) => {
    var fulfilled = (value) => {
      try {
        step(generator.next(value));
      } catch (e) {
        reject(e);
      }
    };
    var rejected = (value) => {
      try {
        step(generator.throw(value));
      } catch (e) {
        reject(e);
      }
    };
    var step = (x) => x.done ? resolve(x.value) : Promise.resolve(x.value).then(fulfilled, rejected);
    step((generator = generator.apply(__this, __arguments)).next());
  });
};
(function() {
  "use strict";
  const isMixed = (value) => typeof value === "symbol";
  const toHex = (color) => {
    const clamp = (value) => Math.min(255, Math.max(0, Math.round(value * 255)));
    const [r, g, b] = [clamp(color.r), clamp(color.g), clamp(color.b)];
    return `#${[r, g, b].map((v) => v.toString(16).padStart(2, "0")).join("")}`;
  };
  const serializeGradientStops = (stops) => stops.map((stop) => ({
    color: toHex(stop.color),
    opacity: stop.color.a,
    position: stop.position
  }));
  const serializePaints = (paints) => {
    if (isMixed(paints)) return "mixed";
    if (!paints || !Array.isArray(paints)) return [];
    return paints.filter((paint) => paint.visible !== false).flatMap((paint) => {
      switch (paint.type) {
        case "SOLID":
          return [
            {
              type: "SOLID",
              color: toHex(paint.color),
              opacity: paint.opacity
            }
          ];
        case "GRADIENT_LINEAR":
        case "GRADIENT_RADIAL":
        case "GRADIENT_ANGULAR":
        case "GRADIENT_DIAMOND":
          return [
            {
              type: paint.type,
              gradientStops: serializeGradientStops(paint.gradientStops),
              gradientTransform: paint.gradientTransform,
              opacity: paint.opacity
            }
          ];
        case "IMAGE":
          return [
            {
              type: "IMAGE",
              scaleMode: paint.scaleMode,
              imageHash: paint.imageHash,
              imageTransform: paint.imageTransform,
              opacity: paint.opacity
            }
          ];
        default:
          return [];
      }
    });
  };
  const serializeEffects = (effects) => effects.filter((effect) => effect.visible !== false).flatMap((effect) => {
    switch (effect.type) {
      case "DROP_SHADOW":
      case "INNER_SHADOW":
        return [
          {
            type: effect.type,
            color: toHex(effect.color),
            opacity: effect.color.a,
            offset: effect.offset,
            radius: effect.radius,
            spread: effect.spread,
            blendMode: effect.blendMode
          }
        ];
      case "LAYER_BLUR":
      case "BACKGROUND_BLUR":
        return [{ type: effect.type, radius: effect.radius }];
      default:
        return [];
    }
  });
  const serializeLineHeight = (lineHeight) => {
    if (isMixed(lineHeight)) return "mixed";
    if ("value" in lineHeight) {
      return { value: lineHeight.value, unit: lineHeight.unit };
    }
    return { unit: lineHeight.unit };
  };
  const serializeLetterSpacing = (letterSpacing) => {
    if (isMixed(letterSpacing)) return "mixed";
    return { value: letterSpacing.value, unit: letterSpacing.unit };
  };
  const getBounds = (node) => {
    if ("x" in node && "y" in node && "width" in node && "height" in node) {
      return {
        x: node.x,
        y: node.y,
        width: node.width,
        height: node.height
      };
    }
    return void 0;
  };
  const serializeText = (node, base) => {
    let fontFamily;
    let fontStyle;
    if (typeof node.fontName === "symbol") {
      fontFamily = "mixed";
      fontStyle = "mixed";
    } else if (node.fontName) {
      fontFamily = node.fontName.family;
      fontStyle = node.fontName.style;
    }
    return __spreadProps(__spreadValues({}, base), {
      characters: node.characters,
      styles: __spreadProps(__spreadValues({}, base.styles), {
        fontSize: isMixed(node.fontSize) ? "mixed" : node.fontSize,
        fontFamily,
        fontStyle,
        fontWeight: isMixed(node.fontWeight) ? "mixed" : node.fontWeight,
        textDecoration: isMixed(node.textDecoration) ? "mixed" : node.textDecoration,
        lineHeight: serializeLineHeight(node.lineHeight),
        letterSpacing: serializeLetterSpacing(node.letterSpacing),
        textAlignHorizontal: isMixed(node.textAlignHorizontal) ? "mixed" : node.textAlignHorizontal,
        textAlignVertical: isMixed(node.textAlignVertical) ? "mixed" : node.textAlignVertical,
        textAutoResize: node.textAutoResize
      })
    });
  };
  const serializeStyles = (node) => {
    const styles = {};
    if ("opacity" in node) {
      styles.opacity = node.opacity;
    }
    if ("blendMode" in node) {
      styles.blendMode = node.blendMode;
    }
    if ("visible" in node) {
      styles.visible = node.visible;
    }
    if ("fills" in node) {
      styles.fills = serializePaints(node.fills);
    }
    if ("strokes" in node) {
      styles.strokes = serializePaints(node.strokes);
    }
    if ("strokeWeight" in node) {
      styles.strokeWeight = isMixed(node.strokeWeight) ? "mixed" : node.strokeWeight;
    }
    if ("strokeAlign" in node) {
      styles.strokeAlign = node.strokeAlign;
    }
    if ("dashPattern" in node) {
      const pattern = node.dashPattern;
      if (pattern.length > 0) {
        styles.dashPattern = [...pattern];
      }
    }
    if ("effects" in node) {
      const effects = node.effects;
      if (effects.length > 0) {
        styles.effects = serializeEffects(effects);
      }
    }
    if ("cornerRadius" in node) {
      styles.cornerRadius = isMixed(node.cornerRadius) ? "mixed" : node.cornerRadius;
    }
    if ("topLeftRadius" in node) {
      const tl = node.topLeftRadius;
      const tr = node.topRightRadius;
      const br = node.bottomRightRadius;
      const bl = node.bottomLeftRadius;
      if (tl !== tr || tr !== br || br !== bl) {
        styles.cornerRadii = {
          topLeft: tl,
          topRight: tr,
          bottomRight: br,
          bottomLeft: bl
        };
      }
    }
    if ("cornerSmoothing" in node) {
      const smoothing = node.cornerSmoothing;
      if (smoothing > 0) {
        styles.cornerSmoothing = smoothing;
      }
    }
    if ("layoutMode" in node) {
      const mode = node.layoutMode;
      if (mode !== "NONE") {
        styles.autoLayout = {
          direction: mode,
          gap: node.itemSpacing,
          primaryAxisAlign: node.primaryAxisAlignItems,
          counterAxisAlign: node.counterAxisAlignItems,
          primaryAxisSizing: node.primaryAxisSizingMode,
          counterAxisSizing: node.counterAxisSizingMode,
          wrap: "layoutWrap" in node ? node.layoutWrap : void 0,
          counterAxisSpacing: "counterAxisSpacing" in node ? node.counterAxisSpacing : void 0
        };
      }
    }
    if ("paddingLeft" in node) {
      const top = node.paddingTop;
      const right = node.paddingRight;
      const bottom = node.paddingBottom;
      const left = node.paddingLeft;
      if (top > 0 || right > 0 || bottom > 0 || left > 0) {
        styles.padding = { top, right, bottom, left };
      }
    }
    if ("clipsContent" in node) {
      styles.clipsContent = node.clipsContent;
    }
    if ("rotation" in node) {
      const rotation = node.rotation;
      if (rotation !== 0) {
        styles.rotation = rotation;
      }
    }
    if ("constraints" in node) {
      const c = node.constraints;
      styles.constraints = { horizontal: c.horizontal, vertical: c.vertical };
    }
    return styles;
  };
  const serializeNode = (node) => {
    const base = {
      id: node.id,
      name: node.name,
      type: node.type,
      bounds: getBounds(node),
      styles: serializeStyles(node)
    };
    if (node.type === "TEXT") {
      return serializeText(node, base);
    }
    if ("children" in node) {
      return __spreadProps(__spreadValues({}, base), {
        children: node.children.filter((child) => child.visible !== false).map((child) => serializeNode(child))
      });
    }
    return base;
  };
  let cachedFallbackFileKey = null;
  const generateFallbackFileKey = () => {
    const random = Math.random().toString(36).slice(2, 10);
    return `unsaved-${Date.now().toString(36)}-${random}`;
  };
  const getFileKey = () => {
    try {
      if (typeof figma.fileKey === "string" && figma.fileKey) {
        return figma.fileKey;
      }
    } catch (e) {
    }
    if (!cachedFallbackFileKey) {
      cachedFallbackFileKey = generateFallbackFileKey();
      console.warn(
        `[figma-mcp-bridge] figma.fileKey unavailable for "${figma.root.name}". Using session fallback key "${cachedFallbackFileKey}". If you encounter this in a built plugin, please report at https://github.com/gethopp/figma-mcp-bridge/issues with steps to reproduce.`
      );
    }
    return cachedFallbackFileKey;
  };
  const sendStatus = () => {
    figma.ui.postMessage({
      type: "plugin-status",
      payload: {
        fileName: figma.root.name,
        fileKey: getFileKey(),
        selectionCount: figma.currentPage.selection.length
      }
    });
  };
  const serializeVariableValue = (value) => {
    if (typeof value === "object" && value !== null) {
      if ("type" in value && value.type === "VARIABLE_ALIAS") {
        return { type: "VARIABLE_ALIAS", id: value.id };
      }
      if ("r" in value && "g" in value && "b" in value) {
        const color = value;
        return {
          type: "COLOR",
          r: color.r,
          g: color.g,
          b: color.b,
          a: "a" in color ? color.a : 1
        };
      }
    }
    return value;
  };
  const isSceneNode = (node) => node !== null && node.type !== "DOCUMENT" && node.type !== "PAGE";
  const isTextNode = (node) => node !== null && node.type === "TEXT";
  const getSceneNodeById = (nodeId) => __async(this, null, function* () {
    const node = yield figma.getNodeByIdAsync(nodeId);
    if (!isSceneNode(node)) {
      throw new Error(`Node not found: ${nodeId}`);
    }
    return node;
  });
  const getTextNodeById = (nodeId) => __async(this, null, function* () {
    const node = yield figma.getNodeByIdAsync(nodeId);
    if (!isTextNode(node)) {
      throw new Error(`Text node not found: ${nodeId}`);
    }
    return node;
  });
  const supportsChildren = (node) => "appendChild" in node;
  const getParentNodeById = (parentId) => __async(this, null, function* () {
    const parent = yield figma.getNodeByIdAsync(parentId);
    if (!parent || parent.type === "DOCUMENT" || !supportsChildren(parent)) {
      throw new Error(`Parent does not support children: ${parentId}`);
    }
    return parent;
  });
  const parseHexColor = (hex) => {
    const normalized = hex.trim().replace(/^#/, "");
    if (normalized.length !== 3 && normalized.length !== 6) {
      throw new Error(`Invalid hex color: ${hex}`);
    }
    const expanded = normalized.length === 3 ? normalized.split("").map((char) => `${char}${char}`).join("") : normalized;
    if (!/^[0-9a-fA-F]{6}$/.test(expanded)) {
      throw new Error(`Invalid hex color: ${hex}`);
    }
    return {
      r: parseInt(expanded.slice(0, 2), 16) / 255,
      g: parseInt(expanded.slice(2, 4), 16) / 255,
      b: parseInt(expanded.slice(4, 6), 16) / 255
    };
  };
  const setSolidFill = (node, fillHex, fillOpacity, target = "fill") => {
    const paint = {
      type: "SOLID",
      color: parseHexColor(fillHex),
      opacity: fillOpacity != null ? fillOpacity : 1
    };
    if (target === "stroke") {
      if (!("strokes" in node)) {
        throw new Error(`Node does not support strokes: ${node.id}`);
      }
      node.strokes = [paint];
      return;
    }
    if (!("fills" in node)) {
      throw new Error(`Node does not support fills: ${node.id}`);
    }
    node.fills = [paint];
  };
  const buildGradientPaint = (paintType, stops, transform, opacity) => {
    const colorStops = stops.map((stop) => {
      var _a;
      const rgb = parseHexColor(stop.hex);
      return {
        position: stop.position,
        color: { r: rgb.r, g: rgb.g, b: rgb.b, a: (_a = stop.opacity) != null ? _a : 1 }
      };
    });
    const gradientTransform = transform != null ? transform : [
      [1, 0, 0],
      [0, 1, 0]
    ];
    const paint = {
      type: paintType,
      gradientStops: colorStops,
      gradientTransform,
      opacity: opacity != null ? opacity : 1
    };
    return paint;
  };
  const loadFontsForTextNode = (node) => __async(this, null, function* () {
    const fonts = /* @__PURE__ */ new Map();
    if (node.characters.length > 0) {
      for (const font of node.getRangeAllFontNames(0, node.characters.length)) {
        fonts.set(`${font.family}::${font.style}`, font);
      }
    } else if (typeof node.fontName !== "symbol") {
      fonts.set(`${node.fontName.family}::${node.fontName.style}`, node.fontName);
    } else {
      throw new Error(
        `Cannot determine font for empty mixed-font text node: ${node.id}`
      );
    }
    yield Promise.all([...fonts.values()].map((font) => figma.loadFontAsync(font)));
  });
  const ensureFont = (family, style) => __async(this, null, function* () {
    const font = { family, style };
    yield figma.loadFontAsync(font);
    return font;
  });
  const applyTextFill = (node, fillHex, fillOpacity) => {
    node.fills = [
      {
        type: "SOLID",
        color: parseHexColor(fillHex),
        opacity: fillOpacity != null ? fillOpacity : 1
      }
    ];
  };
  const positionNode = (node, x, y) => {
    if ("x" in node && typeof x === "number") {
      node.x = x;
    }
    if ("y" in node && typeof y === "number") {
      node.y = y;
    }
  };
  const resizeNodeIfSupported = (node, width, height) => {
    if (typeof width !== "number" && typeof height !== "number") {
      return;
    }
    if (!("resize" in node) || typeof node.resize !== "function") {
      throw new Error(`Node does not support resizing: ${node.id}`);
    }
    const nextWidth = typeof width === "number" ? width : node.width;
    const nextHeight = typeof height === "number" ? height : node.height;
    node.resize(nextWidth, nextHeight);
  };
  const appendToParentIfProvided = (node, parentId) => __async(this, null, function* () {
    if (typeof parentId !== "string") {
      return;
    }
    const parent = yield getParentNodeById(parentId);
    parent.appendChild(node);
  });
  const decodeBase64ToBytes = (base64) => {
    try {
      return figma.base64Decode(base64);
    } catch (e) {
      throw new Error("Invalid base64 image payload");
    }
  };
  const EDIT_REQUEST_TYPES = /* @__PURE__ */ new Set([
    "set_node_visibility",
    "set_text_content",
    "set_text_properties",
    "set_node_properties",
    "set_solid_fill",
    "set_gradient_fill",
    "set_effects",
    "set_stroke_properties",
    "set_auto_layout",
    "create_frame",
    "create_text",
    "create_shape",
    "create_image",
    "duplicate_nodes",
    "reparent_nodes",
    "group_nodes",
    "ungroup_node",
    "delete_nodes"
  ]);
  const requireEditorMode = (toolName) => {
    if (figma.editorType === "dev") {
      throw new Error(
        `${toolName} requires the plugin to be opened in Figma's design editor (Dev Mode is read-only). Switch to the design editor and re-run.`
      );
    }
  };
  const handleRequest = (request) => __async(this, null, function* () {
    var _a, _b, _c, _d, _e, _f, _g, _h, _i, _j, _k, _l, _m, _n, _o, _p, _q, _r, _s, _t, _u, _v, _w, _x, _y, _z, _A, _B, _C, _D, _E, _F, _G;
    try {
      if (EDIT_REQUEST_TYPES.has(request.type)) {
        requireEditorMode(request.type);
      }
      switch (request.type) {
        case "get_document":
          return {
            type: request.type,
            requestId: request.requestId,
            data: serializeNode(figma.currentPage)
          };
        case "get_selection":
          return {
            type: request.type,
            requestId: request.requestId,
            data: figma.currentPage.selection.map((node) => serializeNode(node))
          };
        case "get_node": {
          const nodeId = request.nodeIds && request.nodeIds[0];
          if (!nodeId) {
            throw new Error("nodeIds is required for get_node");
          }
          const node = yield figma.getNodeByIdAsync(nodeId);
          if (!node || node.type === "DOCUMENT") {
            throw new Error(`Node not found: ${nodeId}`);
          }
          return {
            type: request.type,
            requestId: request.requestId,
            data: serializeNode(node)
          };
        }
        case "get_styles": {
          const [paintStyles, textStyles, effectStyles, gridStyles] = yield Promise.all([
            figma.getLocalPaintStylesAsync(),
            figma.getLocalTextStylesAsync(),
            figma.getLocalEffectStylesAsync(),
            figma.getLocalGridStylesAsync()
          ]);
          return {
            type: request.type,
            requestId: request.requestId,
            data: {
              paints: paintStyles.map((style) => ({
                id: style.id,
                name: style.name,
                paints: style.paints
              })),
              text: textStyles.map((style) => ({
                id: style.id,
                name: style.name,
                fontSize: style.fontSize,
                fontName: style.fontName,
                textDecoration: style.textDecoration,
                lineHeight: style.lineHeight,
                letterSpacing: style.letterSpacing
              })),
              effects: effectStyles.map((style) => ({
                id: style.id,
                name: style.name,
                effects: style.effects
              })),
              grids: gridStyles.map((style) => ({
                id: style.id,
                name: style.name,
                layoutGrids: style.layoutGrids
              }))
            }
          };
        }
        case "get_metadata": {
          return {
            type: request.type,
            requestId: request.requestId,
            data: {
              fileName: figma.root.name,
              currentPageId: figma.currentPage.id,
              currentPageName: figma.currentPage.name,
              pageCount: figma.root.children.length,
              pages: figma.root.children.map((page) => ({
                id: page.id,
                name: page.name
              }))
            }
          };
        }
        case "get_design_context": {
          const depth = typeof ((_a = request.params) == null ? void 0 : _a.depth) === "number" ? request.params.depth : 2;
          const serializeWithDepth = (node, currentDepth) => __async(this, null, function* () {
            var _a2, _b2;
            const serialized = serializeNode(node);
            if (currentDepth >= depth && serialized.children) {
              return __spreadProps(__spreadValues({}, serialized), {
                children: void 0,
                childCount: (_b2 = (_a2 = node.children) == null ? void 0 : _a2.filter(
                  (c) => c.visible !== false
                ).length) != null ? _b2 : 0
              });
            }
            if (serialized.children) {
              const childNodes = yield Promise.all(
                serialized.children.map(
                  (child) => figma.getNodeByIdAsync(child.id)
                )
              );
              const serializedChildren = yield Promise.all(
                childNodes.filter(
                  (n) => n !== null && n.type !== "DOCUMENT" && "visible" in n && n.visible !== false
                ).map((n) => serializeWithDepth(n, currentDepth + 1))
              );
              return __spreadProps(__spreadValues({}, serialized), {
                children: serializedChildren
              });
            }
            return serialized;
          });
          const selection = figma.currentPage.selection;
          const contextNodes = selection.length > 0 ? yield Promise.all(
            selection.map((node) => serializeWithDepth(node, 0))
          ) : [
            yield serializeWithDepth(
              figma.currentPage,
              0
            )
          ];
          return {
            type: request.type,
            requestId: request.requestId,
            data: {
              fileName: figma.root.name,
              currentPage: {
                id: figma.currentPage.id,
                name: figma.currentPage.name
              },
              selectionCount: selection.length,
              context: contextNodes
            }
          };
        }
        case "get_variable_defs": {
          const collections = yield figma.variables.getLocalVariableCollectionsAsync();
          const variableData = yield Promise.all(
            collections.map((collection) => __async(this, null, function* () {
              const variables = yield Promise.all(
                collection.variableIds.map(
                  (id) => figma.variables.getVariableByIdAsync(id)
                )
              );
              return {
                id: collection.id,
                name: collection.name,
                modes: collection.modes.map((mode) => ({
                  modeId: mode.modeId,
                  name: mode.name
                })),
                variables: variables.filter((v) => v !== null).map((variable) => ({
                  id: variable.id,
                  name: variable.name,
                  resolvedType: variable.resolvedType,
                  valuesByMode: Object.fromEntries(
                    Object.entries(variable.valuesByMode).map(
                      ([modeId, value]) => [
                        modeId,
                        serializeVariableValue(value)
                      ]
                    )
                  )
                }))
              };
            }))
          );
          return {
            type: request.type,
            requestId: request.requestId,
            data: {
              collections: variableData
            }
          };
        }
        case "get_screenshot": {
          const format = ((_b = request.params) == null ? void 0 : _b.format) === "SVG" || ((_c = request.params) == null ? void 0 : _c.format) === "PDF" || ((_d = request.params) == null ? void 0 : _d.format) === "JPG" || ((_e = request.params) == null ? void 0 : _e.format) === "PNG" ? request.params.format : "PNG";
          const scale = typeof ((_f = request.params) == null ? void 0 : _f.scale) === "number" ? request.params.scale : 2;
          const clip = ((_g = request.params) == null ? void 0 : _g.clip) === true;
          let targetNodes;
          if (request.nodeIds && request.nodeIds.length > 0) {
            const nodes = yield Promise.all(
              request.nodeIds.map((id) => figma.getNodeByIdAsync(id))
            );
            targetNodes = nodes.filter(
              (node) => node !== null && node.type !== "DOCUMENT" && node.type !== "PAGE"
            );
          } else {
            targetNodes = [...figma.currentPage.selection];
          }
          if (targetNodes.length === 0) {
            throw new Error(
              "No nodes to export. Select nodes or provide nodeIds."
            );
          }
          const exports$1 = yield Promise.all(
            targetNodes.map((node) => __async(this, null, function* () {
              const commonSettings = clip ? { contentsOnly: true, useAbsoluteBounds: true } : {};
              const settings = format === "SVG" ? __spreadValues({ format: "SVG" }, commonSettings) : format === "PDF" ? __spreadValues({ format: "PDF" }, commonSettings) : format === "JPG" ? __spreadValues({
                format: "JPG",
                constraint: { type: "SCALE", value: scale }
              }, commonSettings) : __spreadValues({
                format: "PNG",
                constraint: { type: "SCALE", value: scale }
              }, commonSettings);
              const bytes = yield node.exportAsync(settings);
              const base64 = figma.base64Encode(bytes);
              return {
                nodeId: node.id,
                nodeName: node.name,
                format,
                base64,
                width: node.width,
                height: node.height
              };
            }))
          );
          return {
            type: request.type,
            requestId: request.requestId,
            data: {
              exports: exports$1
            }
          };
        }
        case "set_node_visibility": {
          const rawItems = (_h = request.params) == null ? void 0 : _h.items;
          if (!Array.isArray(rawItems) || rawItems.length === 0) {
            throw new Error("items is required for set_node_visibility");
          }
          const items = rawItems;
          const results = [];
          for (const { nodeId, visible } of items) {
            const node = yield figma.getNodeByIdAsync(nodeId);
            if (!node || node.type === "DOCUMENT" || node.type === "PAGE") {
              results.push({ nodeId, error: `Node not found: ${nodeId}` });
              continue;
            }
            const sceneNode = node;
            const previousVisible = sceneNode.visible;
            sceneNode.visible = visible;
            results.push({ nodeId, previousVisible, visible });
          }
          return {
            type: request.type,
            requestId: request.requestId,
            data: { results }
          };
        }
        case "set_text_content": {
          const nodeId = request.nodeIds && request.nodeIds[0];
          const text = (_i = request.params) == null ? void 0 : _i.text;
          if (!nodeId) {
            throw new Error("nodeIds is required for set_text_content");
          }
          if (typeof text !== "string") {
            throw new Error("text is required for set_text_content");
          }
          const node = yield getTextNodeById(nodeId);
          yield loadFontsForTextNode(node);
          const previousCharacters = node.characters;
          node.characters = text;
          return {
            type: request.type,
            requestId: request.requestId,
            data: {
              nodeId: node.id,
              nodeName: node.name,
              previousCharacters,
              characters: node.characters
            }
          };
        }
        case "set_text_properties": {
          const nodeId = request.nodeIds && request.nodeIds[0];
          if (!nodeId) {
            throw new Error("nodeIds is required for set_text_properties");
          }
          const node = yield getTextNodeById(nodeId);
          const params = (_j = request.params) != null ? _j : {};
          const applied = {};
          yield loadFontsForTextNode(node);
          if (typeof params.fontFamily === "string" || typeof params.fontStyle === "string") {
            const currentFontName = typeof node.fontName === "symbol" ? null : node.fontName;
            const nextFamily = typeof params.fontFamily === "string" ? params.fontFamily : currentFontName == null ? void 0 : currentFontName.family;
            const nextStyle = typeof params.fontStyle === "string" ? params.fontStyle : currentFontName == null ? void 0 : currentFontName.style;
            if (!nextFamily || !nextStyle) {
              throw new Error(
                "fontFamily and fontStyle must resolve to a concrete font for set_text_properties"
              );
            }
            node.fontName = yield ensureFont(nextFamily, nextStyle);
            applied.fontName = node.fontName;
          }
          if (typeof params.fontSize === "number") {
            node.fontSize = params.fontSize;
            applied.fontSize = node.fontSize;
          }
          if (params.textAlignHorizontal === "LEFT" || params.textAlignHorizontal === "CENTER" || params.textAlignHorizontal === "RIGHT" || params.textAlignHorizontal === "JUSTIFIED") {
            node.textAlignHorizontal = params.textAlignHorizontal;
            applied.textAlignHorizontal = node.textAlignHorizontal;
          }
          if (params.textAlignVertical === "TOP" || params.textAlignVertical === "CENTER" || params.textAlignVertical === "BOTTOM") {
            node.textAlignVertical = params.textAlignVertical;
            applied.textAlignVertical = node.textAlignVertical;
          }
          if (params.textAutoResize === "NONE" || params.textAutoResize === "WIDTH_AND_HEIGHT" || params.textAutoResize === "HEIGHT" || params.textAutoResize === "TRUNCATE") {
            node.textAutoResize = params.textAutoResize;
            applied.textAutoResize = node.textAutoResize;
          }
          if (typeof params.lineHeightPx === "number") {
            node.lineHeight = {
              unit: "PIXELS",
              value: params.lineHeightPx
            };
            applied.lineHeight = node.lineHeight;
          }
          if (typeof params.letterSpacingPx === "number") {
            node.letterSpacing = {
              unit: "PIXELS",
              value: params.letterSpacingPx
            };
            applied.letterSpacing = node.letterSpacing;
          }
          if (typeof params.fillHex === "string") {
            const fillOpacity = typeof params.fillOpacity === "number" ? params.fillOpacity : void 0;
            applyTextFill(node, params.fillHex, fillOpacity);
            applied.fillHex = params.fillHex;
            applied.fillOpacity = fillOpacity != null ? fillOpacity : 1;
          }
          if (typeof params.x === "number" || typeof params.y === "number") {
            positionNode(node, params.x, params.y);
            applied.x = node.x;
            applied.y = node.y;
          }
          resizeNodeIfSupported(node, params.width, params.height);
          if (typeof params.width === "number" || typeof params.height === "number") {
            applied.width = node.width;
            applied.height = node.height;
          }
          return {
            type: request.type,
            requestId: request.requestId,
            data: {
              nodeId: node.id,
              nodeName: node.name,
              applied
            }
          };
        }
        case "set_node_properties": {
          const nodeId = request.nodeIds && request.nodeIds[0];
          if (!nodeId) {
            throw new Error("nodeIds is required for set_node_properties");
          }
          const node = yield getSceneNodeById(nodeId);
          const params = (_k = request.params) != null ? _k : {};
          const applied = {};
          const hasUpdates = Object.keys(params).length > 0;
          if (!hasUpdates) {
            throw new Error("At least one property is required for set_node_properties");
          }
          if (typeof params.name === "string") {
            node.name = params.name;
            applied.name = node.name;
          }
          if (typeof params.visible === "boolean") {
            node.visible = params.visible;
            applied.visible = node.visible;
          }
          if (typeof params.x === "number" || typeof params.y === "number") {
            if (!("x" in node) || !("y" in node)) {
              throw new Error(`Node does not support x/y positioning: ${node.id}`);
            }
            positionNode(node, params.x, params.y);
            applied.x = node.x;
            applied.y = node.y;
          }
          if (typeof params.width === "number" || typeof params.height === "number") {
            resizeNodeIfSupported(node, params.width, params.height);
            applied.width = node.width;
            applied.height = node.height;
          }
          if (typeof params.rotation === "number") {
            if (!("rotation" in node)) {
              throw new Error(`Node does not support rotation: ${node.id}`);
            }
            node.rotation = params.rotation;
            applied.rotation = node.rotation;
          }
          if (typeof params.opacity === "number") {
            if (!("opacity" in node)) {
              throw new Error(`Node does not support opacity: ${node.id}`);
            }
            node.opacity = params.opacity;
            applied.opacity = node.opacity;
          }
          if (typeof params.cornerRadius === "number") {
            if (!("cornerRadius" in node)) {
              throw new Error(`Node does not support cornerRadius: ${node.id}`);
            }
            node.cornerRadius = params.cornerRadius;
            applied.cornerRadius = node.cornerRadius;
          }
          return {
            type: request.type,
            requestId: request.requestId,
            data: {
              nodeId: node.id,
              nodeName: node.name,
              applied
            }
          };
        }
        case "set_solid_fill": {
          const nodeId = request.nodeIds && request.nodeIds[0];
          if (!nodeId) {
            throw new Error("nodeIds is required for set_solid_fill");
          }
          const node = yield getSceneNodeById(nodeId);
          const params = (_l = request.params) != null ? _l : {};
          if (typeof params.hex !== "string") {
            throw new Error("hex is required");
          }
          const target = params.target === "stroke" ? "stroke" : "fill";
          const opacity = typeof params.opacity === "number" ? params.opacity : void 0;
          setSolidFill(node, params.hex, opacity, target);
          return {
            type: request.type,
            requestId: request.requestId,
            data: {
              nodeId: node.id,
              nodeName: node.name,
              applied: {
                target,
                hex: params.hex,
                opacity: opacity != null ? opacity : 1
              }
            }
          };
        }
        case "set_gradient_fill": {
          const nodeId = request.nodeIds && request.nodeIds[0];
          if (!nodeId) {
            throw new Error("nodeIds is required for set_gradient_fill");
          }
          const node = yield getSceneNodeById(nodeId);
          const params = (_m = request.params) != null ? _m : {};
          const target = params.target === "stroke" ? "stroke" : "fill";
          if (target === "fill" && !("fills" in node)) {
            throw new Error(`Node does not support fills: ${node.id}`);
          }
          if (target === "stroke" && !("strokes" in node)) {
            throw new Error(`Node does not support strokes: ${node.id}`);
          }
          const gradientType = typeof params.gradientType === "string" ? params.gradientType : "LINEAR";
          const paintType = `GRADIENT_${gradientType}`;
          if (paintType !== "GRADIENT_LINEAR" && paintType !== "GRADIENT_RADIAL" && paintType !== "GRADIENT_ANGULAR" && paintType !== "GRADIENT_DIAMOND") {
            throw new Error(`Unsupported gradient type: ${gradientType}`);
          }
          if (!Array.isArray(params.gradientStops) || params.gradientStops.length < 2) {
            throw new Error("gradientStops must have at least 2 entries");
          }
          const stops = params.gradientStops;
          const transform = Array.isArray(params.gradientTransform) && params.gradientTransform.length === 2 ? params.gradientTransform : void 0;
          const opacity = typeof params.opacity === "number" ? params.opacity : void 0;
          const paint = buildGradientPaint(paintType, stops, transform, opacity);
          if (target === "fill") {
            node.fills = [paint];
          } else {
            node.strokes = [paint];
          }
          return {
            type: request.type,
            requestId: request.requestId,
            data: {
              nodeId: node.id,
              nodeName: node.name,
              applied: {
                target,
                gradientType: paintType,
                stops: paint.gradientStops.length
              }
            }
          };
        }
        case "set_effects": {
          const nodeId = request.nodeIds && request.nodeIds[0];
          if (!nodeId) {
            throw new Error("nodeIds is required for set_effects");
          }
          const node = yield getSceneNodeById(nodeId);
          if (!("effects" in node)) {
            throw new Error(`Node does not support effects: ${node.id}`);
          }
          const params = (_n = request.params) != null ? _n : {};
          if (!Array.isArray(params.effects)) {
            throw new Error("effects must be an array (pass [] to clear)");
          }
          const built = params.effects.map(
            (raw, i) => {
              const type = raw.type;
              if (type === "DROP_SHADOW" || type === "INNER_SHADOW") {
                if (typeof raw.color !== "string") {
                  throw new Error(`effects[${i}].color must be a hex string`);
                }
                const offset = raw.offset;
                if (!offset || typeof offset.x !== "number" || typeof offset.y !== "number") {
                  throw new Error(`effects[${i}].offset must be {x,y} numbers`);
                }
                if (typeof raw.radius !== "number") {
                  throw new Error(`effects[${i}].radius must be a number`);
                }
                const rgb = parseHexColor(raw.color);
                const alpha = typeof raw.opacity === "number" ? raw.opacity : 1;
                return {
                  type,
                  color: { r: rgb.r, g: rgb.g, b: rgb.b, a: alpha },
                  offset: { x: offset.x, y: offset.y },
                  radius: raw.radius,
                  spread: typeof raw.spread === "number" ? raw.spread : 0,
                  visible: raw.visible === void 0 ? true : Boolean(raw.visible),
                  blendMode: typeof raw.blendMode === "string" ? raw.blendMode : "NORMAL"
                };
              }
              if (type === "LAYER_BLUR" || type === "BACKGROUND_BLUR") {
                if (typeof raw.radius !== "number") {
                  throw new Error(`effects[${i}].radius must be a number`);
                }
                return {
                  type,
                  radius: raw.radius,
                  visible: raw.visible === void 0 ? true : Boolean(raw.visible)
                };
              }
              throw new Error(`Unsupported effect type at effects[${i}]: ${String(type)}`);
            }
          );
          node.effects = built;
          return {
            type: request.type,
            requestId: request.requestId,
            data: {
              nodeId: node.id,
              nodeName: node.name,
              applied: { count: built.length }
            }
          };
        }
        case "set_stroke_properties": {
          const nodeId = request.nodeIds && request.nodeIds[0];
          if (!nodeId) {
            throw new Error("nodeIds is required for set_stroke_properties");
          }
          const node = yield getSceneNodeById(nodeId);
          const params = (_o = request.params) != null ? _o : {};
          const applied = {};
          if (typeof params.strokeWeight === "number") {
            if (!("strokeWeight" in node)) {
              throw new Error(`Node does not support strokeWeight: ${node.id}`);
            }
            node.strokeWeight = params.strokeWeight;
            applied.strokeWeight = params.strokeWeight;
          }
          if (params.strokeAlign === "INSIDE" || params.strokeAlign === "OUTSIDE" || params.strokeAlign === "CENTER") {
            if (!("strokeAlign" in node)) {
              throw new Error(`Node does not support strokeAlign: ${node.id}`);
            }
            node.strokeAlign = params.strokeAlign;
            applied.strokeAlign = params.strokeAlign;
          }
          if (Array.isArray(params.dashPattern)) {
            if (!("dashPattern" in node)) {
              throw new Error(`Node does not support dashPattern: ${node.id}`);
            }
            const pattern = params.dashPattern.map((n, i) => {
              if (typeof n !== "number" || n < 0) {
                throw new Error(`dashPattern[${i}] must be a non-negative number`);
              }
              return n;
            });
            node.dashPattern = pattern;
            applied.dashPattern = pattern;
          }
          if (typeof params.strokeCap === "string") {
            if (!("strokeCap" in node)) {
              throw new Error(`Node does not support strokeCap: ${node.id}`);
            }
            node.strokeCap = params.strokeCap;
            applied.strokeCap = params.strokeCap;
          }
          if (typeof params.strokeJoin === "string") {
            if (!("strokeJoin" in node)) {
              throw new Error(`Node does not support strokeJoin: ${node.id}`);
            }
            node.strokeJoin = params.strokeJoin;
            applied.strokeJoin = params.strokeJoin;
          }
          return {
            type: request.type,
            requestId: request.requestId,
            data: {
              nodeId: node.id,
              nodeName: node.name,
              applied
            }
          };
        }
        case "set_auto_layout": {
          const nodeId = request.nodeIds && request.nodeIds[0];
          if (!nodeId) {
            throw new Error("nodeIds is required for set_auto_layout");
          }
          const node = yield getSceneNodeById(nodeId);
          if (!("layoutMode" in node)) {
            throw new Error(`Node does not support auto-layout: ${node.id}`);
          }
          const frame = node;
          const params = (_p = request.params) != null ? _p : {};
          const applied = {};
          if (params.layoutMode === "NONE" || params.layoutMode === "HORIZONTAL" || params.layoutMode === "VERTICAL") {
            frame.layoutMode = params.layoutMode;
            applied.layoutMode = params.layoutMode;
          }
          if (typeof params.itemSpacing === "number") {
            frame.itemSpacing = params.itemSpacing;
            applied.itemSpacing = params.itemSpacing;
          }
          if (typeof params.counterAxisSpacing === "number") {
            frame.counterAxisSpacing = params.counterAxisSpacing;
            applied.counterAxisSpacing = params.counterAxisSpacing;
          }
          if (typeof params.paddingTop === "number") {
            frame.paddingTop = params.paddingTop;
            applied.paddingTop = params.paddingTop;
          }
          if (typeof params.paddingRight === "number") {
            frame.paddingRight = params.paddingRight;
            applied.paddingRight = params.paddingRight;
          }
          if (typeof params.paddingBottom === "number") {
            frame.paddingBottom = params.paddingBottom;
            applied.paddingBottom = params.paddingBottom;
          }
          if (typeof params.paddingLeft === "number") {
            frame.paddingLeft = params.paddingLeft;
            applied.paddingLeft = params.paddingLeft;
          }
          if (params.primaryAxisAlignItems === "MIN" || params.primaryAxisAlignItems === "MAX" || params.primaryAxisAlignItems === "CENTER" || params.primaryAxisAlignItems === "SPACE_BETWEEN") {
            frame.primaryAxisAlignItems = params.primaryAxisAlignItems;
            applied.primaryAxisAlignItems = params.primaryAxisAlignItems;
          }
          if (params.counterAxisAlignItems === "MIN" || params.counterAxisAlignItems === "MAX" || params.counterAxisAlignItems === "CENTER" || params.counterAxisAlignItems === "BASELINE") {
            frame.counterAxisAlignItems = params.counterAxisAlignItems;
            applied.counterAxisAlignItems = params.counterAxisAlignItems;
          }
          if (params.primaryAxisSizingMode === "FIXED" || params.primaryAxisSizingMode === "AUTO") {
            frame.primaryAxisSizingMode = params.primaryAxisSizingMode;
            applied.primaryAxisSizingMode = params.primaryAxisSizingMode;
          }
          if (params.counterAxisSizingMode === "FIXED" || params.counterAxisSizingMode === "AUTO") {
            frame.counterAxisSizingMode = params.counterAxisSizingMode;
            applied.counterAxisSizingMode = params.counterAxisSizingMode;
          }
          if (params.layoutWrap === "NO_WRAP" || params.layoutWrap === "WRAP") {
            frame.layoutWrap = params.layoutWrap;
            applied.layoutWrap = params.layoutWrap;
          }
          return {
            type: request.type,
            requestId: request.requestId,
            data: {
              nodeId: node.id,
              nodeName: node.name,
              applied
            }
          };
        }
        case "create_frame": {
          const params = (_q = request.params) != null ? _q : {};
          const frame = figma.createFrame();
          if (typeof params.name === "string") {
            frame.name = params.name;
          }
          const width = typeof params.width === "number" ? params.width : 100;
          const height = typeof params.height === "number" ? params.height : 100;
          frame.resize(width, height);
          if (typeof params.fillHex === "string") {
            const fillOpacity = typeof params.fillOpacity === "number" ? params.fillOpacity : void 0;
            setSolidFill(frame, params.fillHex, fillOpacity);
          }
          yield appendToParentIfProvided(frame, params.parentId);
          positionNode(frame, params.x, params.y);
          return {
            type: request.type,
            requestId: request.requestId,
            data: {
              nodeId: frame.id,
              nodeName: frame.name,
              parentId: (_r = frame.parent) == null ? void 0 : _r.id,
              x: frame.x,
              y: frame.y,
              width: frame.width,
              height: frame.height
            }
          };
        }
        case "create_text": {
          const params = (_s = request.params) != null ? _s : {};
          const text = figma.createText();
          const fontFamily = typeof params.fontFamily === "string" ? params.fontFamily : "Inter";
          const fontStyle = typeof params.fontStyle === "string" ? params.fontStyle : "Regular";
          text.fontName = yield ensureFont(fontFamily, fontStyle);
          if (typeof params.name === "string") {
            text.name = params.name;
          }
          if (typeof params.characters === "string") {
            text.characters = params.characters;
          }
          if (typeof params.fontSize === "number") {
            text.fontSize = params.fontSize;
          }
          if (typeof params.fillHex === "string") {
            const fillOpacity = typeof params.fillOpacity === "number" ? params.fillOpacity : void 0;
            applyTextFill(text, params.fillHex, fillOpacity);
          }
          if (params.textAlignHorizontal === "LEFT" || params.textAlignHorizontal === "CENTER" || params.textAlignHorizontal === "RIGHT" || params.textAlignHorizontal === "JUSTIFIED") {
            text.textAlignHorizontal = params.textAlignHorizontal;
          }
          if (params.textAutoResize === "NONE" || params.textAutoResize === "WIDTH_AND_HEIGHT" || params.textAutoResize === "HEIGHT" || params.textAutoResize === "TRUNCATE") {
            text.textAutoResize = params.textAutoResize;
          }
          resizeNodeIfSupported(text, params.width, params.height);
          yield appendToParentIfProvided(text, params.parentId);
          positionNode(text, params.x, params.y);
          return {
            type: request.type,
            requestId: request.requestId,
            data: {
              nodeId: text.id,
              nodeName: text.name,
              parentId: (_t = text.parent) == null ? void 0 : _t.id,
              characters: text.characters,
              x: text.x,
              y: text.y,
              width: text.width,
              height: text.height
            }
          };
        }
        case "create_shape": {
          const params = (_u = request.params) != null ? _u : {};
          const shapeType = params.shapeType;
          let node;
          if (shapeType === "ELLIPSE") {
            node = figma.createEllipse();
          } else if (shapeType === "LINE") {
            node = figma.createLine();
          } else {
            node = figma.createRectangle();
          }
          if (typeof params.name === "string") {
            node.name = params.name;
          }
          resizeNodeIfSupported(node, params.width, params.height);
          if (typeof params.rotation === "number" && "rotation" in node) {
            node.rotation = params.rotation;
          }
          if (shapeType === "LINE" && typeof params.fillHex === "string") {
            throw new Error("LINE shapes do not support fillHex — use strokeHex instead");
          }
          if (typeof params.fillHex === "string") {
            const fillOpacity = typeof params.fillOpacity === "number" ? params.fillOpacity : void 0;
            setSolidFill(node, params.fillHex, fillOpacity);
          }
          if (shapeType === "LINE" && typeof params.strokeHex !== "string") {
            throw new Error(
              "LINE shapes require strokeHex (lines have no fill, so without a stroke they are invisible)"
            );
          }
          if (typeof params.strokeHex === "string") {
            if (!("strokes" in node)) {
              throw new Error(`Node does not support strokes: ${node.id}`);
            }
            const strokeOpacity = typeof params.strokeOpacity === "number" ? params.strokeOpacity : void 0;
            setSolidFill(node, params.strokeHex, strokeOpacity, "stroke");
          }
          if ("strokeWeight" in node && typeof params.strokeWeight === "number") {
            node.strokeWeight = params.strokeWeight;
          }
          if (typeof params.cornerRadius === "number" && "cornerRadius" in node) {
            node.cornerRadius = params.cornerRadius;
          }
          yield appendToParentIfProvided(node, params.parentId);
          positionNode(node, params.x, params.y);
          return {
            type: request.type,
            requestId: request.requestId,
            data: {
              nodeId: node.id,
              nodeName: node.name,
              shapeType,
              parentId: (_v = node.parent) == null ? void 0 : _v.id,
              x: "x" in node ? node.x : void 0,
              y: "y" in node ? node.y : void 0,
              width: "width" in node ? node.width : void 0,
              height: "height" in node ? node.height : void 0
            }
          };
        }
        case "create_image": {
          const params = (_w = request.params) != null ? _w : {};
          if (typeof params.imageBase64 !== "string" || params.imageBase64.length === 0) {
            throw new Error("imageBase64 is required for create_image");
          }
          const image = figma.createImage(decodeBase64ToBytes(params.imageBase64));
          const imageSize = yield image.getSizeAsync();
          const node = figma.createRectangle();
          if (typeof params.name === "string") {
            node.name = params.name;
          }
          const aspectRatio = imageSize.width / imageSize.height;
          const width = typeof params.width === "number" ? params.width : typeof params.height === "number" ? params.height * aspectRatio : imageSize.width;
          const height = typeof params.height === "number" ? params.height : typeof params.width === "number" ? params.width / aspectRatio : imageSize.height;
          node.resize(width, height);
          node.fills = [
            {
              type: "IMAGE",
              imageHash: image.hash,
              scaleMode: params.scaleMode === "FIT" ? "FIT" : "FILL"
            }
          ];
          if (typeof params.cornerRadius === "number") {
            node.cornerRadius = params.cornerRadius;
          }
          yield appendToParentIfProvided(node, params.parentId);
          positionNode(node, params.x, params.y);
          return {
            type: request.type,
            requestId: request.requestId,
            data: {
              nodeId: node.id,
              nodeName: node.name,
              parentId: (_x = node.parent) == null ? void 0 : _x.id,
              x: node.x,
              y: node.y,
              width: node.width,
              height: node.height,
              imageHash: image.hash
            }
          };
        }
        case "duplicate_nodes": {
          if (!request.nodeIds || request.nodeIds.length === 0) {
            throw new Error("nodeIds is required for duplicate_nodes");
          }
          const duplicates = [];
          for (const nodeId of request.nodeIds) {
            const node = yield getSceneNodeById(nodeId);
            if (!("clone" in node) || typeof node.clone !== "function") {
              throw new Error(`Node does not support duplication: ${node.id}`);
            }
            const clone = node.clone();
            duplicates.push({
              sourceNodeId: node.id,
              nodeId: clone.id,
              nodeName: clone.name,
              parentId: (_y = clone.parent) == null ? void 0 : _y.id
            });
          }
          return {
            type: request.type,
            requestId: request.requestId,
            data: {
              duplicatedCount: duplicates.length,
              duplicates
            }
          };
        }
        case "reparent_nodes": {
          if (!request.nodeIds || request.nodeIds.length === 0) {
            throw new Error("nodeIds is required for reparent_nodes");
          }
          const parentId = (_z = request.params) == null ? void 0 : _z.parentId;
          if (typeof parentId !== "string") {
            throw new Error("parentId is required for reparent_nodes");
          }
          const parent = yield getParentNodeById(parentId);
          const moved = [];
          for (const nodeId of request.nodeIds) {
            const node = yield getSceneNodeById(nodeId);
            parent.appendChild(node);
            moved.push({
              nodeId: node.id,
              nodeName: node.name,
              parentId: (_A = node.parent) == null ? void 0 : _A.id
            });
          }
          return {
            type: request.type,
            requestId: request.requestId,
            data: {
              movedCount: moved.length,
              moved
            }
          };
        }
        case "group_nodes": {
          if (!request.nodeIds || request.nodeIds.length === 0) {
            throw new Error("nodeIds is required for group_nodes");
          }
          const nodes = yield Promise.all(
            request.nodeIds.map((nodeId) => getSceneNodeById(nodeId))
          );
          const explicitParentId = (_B = request.params) == null ? void 0 : _B.parentId;
          let parent;
          if (typeof explicitParentId === "string") {
            parent = yield getParentNodeById(explicitParentId);
          } else {
            const parents = new Set(nodes.map((n) => {
              var _a2;
              return (_a2 = n.parent) == null ? void 0 : _a2.id;
            }));
            if (parents.size !== 1 || parents.has(void 0)) {
              throw new Error(
                "group_nodes requires all nodes to share a parent, or pass parentId explicitly"
              );
            }
            const sharedParent = nodes[0].parent;
            if (!sharedParent || !supportsChildren(sharedParent)) {
              throw new Error("Shared parent does not support children");
            }
            parent = sharedParent;
          }
          const group = figma.group(nodes, parent);
          const name = (_C = request.params) == null ? void 0 : _C.name;
          if (typeof name === "string") {
            group.name = name;
          }
          return {
            type: request.type,
            requestId: request.requestId,
            data: {
              nodeId: group.id,
              nodeName: group.name,
              parentId: (_D = group.parent) == null ? void 0 : _D.id,
              childIds: group.children.map((c) => c.id)
            }
          };
        }
        case "ungroup_node": {
          const nodeId = request.nodeIds && request.nodeIds[0];
          if (!nodeId) {
            throw new Error("nodeIds is required for ungroup_node");
          }
          const node = yield getSceneNodeById(nodeId);
          if (node.type !== "GROUP" && node.type !== "FRAME") {
            throw new Error(
              `ungroup_node only works on GROUP or FRAME nodes, got ${node.type}`
            );
          }
          const parentId = (_E = node.parent) == null ? void 0 : _E.id;
          const orphans = figma.ungroup(node);
          return {
            type: request.type,
            requestId: request.requestId,
            data: {
              parentId,
              orphanIds: orphans.map((o) => o.id)
            }
          };
        }
        case "set_selection": {
          const ids = (_F = request.nodeIds) != null ? _F : [];
          const nodes = [];
          for (const id of ids) {
            nodes.push(yield getSceneNodeById(id));
          }
          figma.currentPage.selection = nodes;
          return {
            type: request.type,
            requestId: request.requestId,
            data: {
              selectedCount: nodes.length,
              selectedIds: nodes.map((n) => n.id)
            }
          };
        }
        case "scroll_and_zoom_into_view": {
          if (!request.nodeIds || request.nodeIds.length === 0) {
            throw new Error("nodeIds is required for scroll_and_zoom_into_view");
          }
          const nodes = yield Promise.all(
            request.nodeIds.map((nodeId) => getSceneNodeById(nodeId))
          );
          figma.viewport.scrollAndZoomIntoView(nodes);
          return {
            type: request.type,
            requestId: request.requestId,
            data: {
              framedCount: nodes.length,
              framedIds: nodes.map((n) => n.id)
            }
          };
        }
        case "delete_nodes": {
          if (((_G = request.params) == null ? void 0 : _G.confirm) !== true) {
            throw new Error("delete_nodes requires confirm: true");
          }
          if (!request.nodeIds || request.nodeIds.length === 0) {
            throw new Error("nodeIds is required for delete_nodes");
          }
          const nodes = yield Promise.all(request.nodeIds.map((nodeId) => getSceneNodeById(nodeId)));
          const deletions = nodes.map((node) => {
            var _a2;
            return {
              nodeId: node.id,
              nodeName: node.name,
              parentId: (_a2 = node.parent) == null ? void 0 : _a2.id
            };
          });
          for (const node of nodes) {
            node.remove();
          }
          return {
            type: request.type,
            requestId: request.requestId,
            data: {
              deletedCount: deletions.length,
              deletions
            }
          };
        }
        default:
          throw new Error(`Unknown request type: ${request.type}`);
      }
    } catch (error) {
      return {
        type: request.type,
        requestId: request.requestId,
        error: error instanceof Error ? error.message : String(error)
      };
    }
  });
  figma.showUI(__html__, { width: 320, height: 180 });
  sendStatus();
  figma.on("selectionchange", () => {
    sendStatus();
  });
  figma.ui.onmessage = (message) => __async(this, null, function* () {
    if (message.type === "ui-ready") {
      sendStatus();
      return;
    }
    if (message.type === "server-request") {
      const response = yield handleRequest(message.payload);
      try {
        figma.ui.postMessage(response);
      } catch (err) {
        figma.ui.postMessage({
          type: response.type,
          requestId: response.requestId,
          error: err instanceof Error ? err.message : String(err)
        });
      }
    }
  });
})();
