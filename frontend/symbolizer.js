/* Copyright 2013 Google Inc. All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

angular.module('crsym', [])
  .config(['$interpolateProvider', function($interpolateProvider) {
    // Change the interpolation symbols because they conflict with Go's
    // template language.
    $interpolateProvider.startSymbol('{@');
    $interpolateProvider.endSymbol('@}');
  }])
  .controller('CrsymController', ['$scope', '$http', function($scope, $http) {
    /** The current input type. */
    $scope.inputType = 'apple';

    /** The data to symbolize. */
    $scope.input = '';

    /**
     * Type-specific data. Object of objects, keyed by inputType, that contain
     * additional key-value paris.
     */
    $scope.typeData = {};

    /** Whether or not a backend request is in progress. */
    $scope.inProgress = false;

    /** The symbolization output data. */
    $scope.output = '';

    /** Whether the symbolization failed. */
    $scope.error = false;

    /**
     * Determines if the selected input type requires the large input field.
     */
    $scope.hideInputArea = function() {
      return $scope.inputType == 'crash_key' ||
             $scope.inputType == 'module_info';
    };

    /**
     * Performs the actual symbolization work.
     */
    $scope.symbolize = function() {
      $scope.processing = true;
      $scope.error = false;
      $scope.output = 'Processing\u2026\n\nThis may take up to 60 seconds.';
      window.location.hash = 'output';

      var data = $scope.typeData[$scope.inputType] || {};
      data.input_type = $scope.inputType;
      data.input = $scope.input;

      var config = {
        method: 'POST',
        url: '/_/service',
        data: data,
        headers: {'Content-Type': 'application/x-www-form-urlencoded'},
        transformRequest: function(data) {
          var result = '';
          for (var key in data) {
            result += encodeURIComponent(key) + '=' +
                      encodeURIComponent(data[key]) + '&';
          }
          return result;
        }
      };
      $http(config)
        .success(function(data) {
          $scope.output = data;
        })
        .error(function(data) {
          $scope.error = true;
          $scope.output = data;
        })
        .then(function() {
          $scope.processing = false;
        });
    };
  }]);
