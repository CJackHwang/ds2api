'use strict';

const XML_TOOL_SEGMENT_TAGS = [
  '<tool_calls>', '<tool_calls\n', '<tool_calls ', '<tool_call>', '<tool_call\n', '<tool_call ',
  '<invoke ', '<invoke>', '<function_call', '<function_calls', '<tool_use>',
];

const XML_TOOL_OPENING_TAGS = [
  '<tool_calls', '<tool_call', '<invoke', '<function_call', '<function_calls', '<tool_use',
];

const XML_TOOL_CLOSING_TAGS = [
  '</tool_calls>', '</tool_call>', '</invoke>', '</function_call>', '</function_calls>', '</tool_use>',
];

module.exports = {
  XML_TOOL_SEGMENT_TAGS,
  XML_TOOL_OPENING_TAGS,
  XML_TOOL_CLOSING_TAGS,
};

