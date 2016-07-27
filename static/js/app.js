var loraserver = angular.module('loraserver', [
    'ngRoute',

    'loraserverControllers'
    ]);

loraserver.directive('stringToNumber', function() {
  return {
    require: 'ngModel',
    link: function(scope, element, attrs, ngModel) {
      ngModel.$parsers.push(function(value) {
        return '' + value;
      });
      ngModel.$formatters.push(function(value) {
        return parseFloat(value);
      });
    }
  };
});

loraserver.config(['$routeProvider',
    function($routeProvider) {
        $routeProvider.
            when('/applications', {
                templateUrl: 'partials/applications.html',
                controller: 'ApplicationListCtrl'
            }).
            when('/applications/:application', {
                templateUrl: 'partials/application.html',
                controller: 'ApplicationCtrl'
            }).
            when('/channels', {
                templateUrl: 'partials/channel_lists.html',
                controller: 'ChannelListListCtrl'
            }).
            when('/channels/:list', {
                templateUrl: 'partials/channel_list.html',
                controller: 'ChannelListCtrl'
            }).
            otherwise({
                redirectTo: '/applications'
            });
        }]);

var loraserverControllers = angular.module('loraserverControllers', []);

// manage applications
loraserverControllers.controller('ApplicationListCtrl', ['$scope', '$http', '$routeParams', '$route',
    function ($scope, $http, $routeParams, $route) {
        $scope.page = 'applications';
        $http.get('/api/v1/application/0/9999').success(function(data) {
                $scope.apps = data.result;
        });

        $scope.createApplication = function(app) {
            if(app == null) {
                $('#createModal').modal().on('hidden.bs.modal', function() {
                    $route.reload();
                });
            } else {
                $http.post('/api/v1/application', app).success(function(data) {
                    $('#createModal').modal('hide');
                }).error(function(data) {
                    $scope.error = data.Error;
				});
            }
        };

        $scope.editApplication = function(app) {
            $http.get('/api/v1/application/' + app.appEUI).success(function(data) {
                $scope.app = data;
                $('#editApplicationModal').modal().on('hidden.bs.modal', function() {
                    $route.reload();
                });
            });
        };

        $scope.updateApplication = function(app) {
            $http.put('/api/v1/application/' + app.appEUI, app).success(function(data) {
                $('#editApplicationModal').modal('hide');
			}).error(function(data) {
                $scope.error = data.Error;
            });
        };

        $scope.deleteApplication = function(app) {
            if (confirm('Are you sure you want to delete ' + app.appEUI + '?')) {
                $http.delete('/api/v1/application/' + app.appEUI).success(function(data) {
                    $route.reload();
                }).error(function(data) {
					alert(data.Error);
				});
            }
        };
    }]);

// manage nodes
loraserverControllers.controller('ApplicationCtrl', ['$scope', '$http', '$routeParams', '$route',
    function ($scope, $http, $routeParams, $route) {
        $scope.page = 'applications';

        $http.get('/api/v1/application/' + $routeParams.application).success(function(data) {
            $scope.application = data;
        }).error(function(data) {alert(data.Error)});

        $http.get('/api/v1/node/application/' + $routeParams.application + '/0/9999').success(function(data) {
            $scope.nodes = data.result;
        }).error(function(data) {alert(data.Error)});

        $http.get('/api/v1/channelList/0/9999').success(function(data) {
            $scope.channelLists = data.result;
        }).error(function(data) {alert(data.Error)});

        $scope.createNode = function(node) {
            if (node == null) {
                $('#createNodeModal').modal().on('hidden.bs.modal', function(){
                    $route.reload();
                });
            } else {
                node.appEUI = $routeParams.application;
                $http.post('/api/v1/node', node).success(function(data) {
                    $('#createNodeModal').modal('hide');
				}).error(function(data) {
                    $scope.error = data.Error;
                });
            }
        };

        $scope.editNode = function(node) {
            $http.get('/api/v1/node/' + node.devEUI).success(function(data) {
                $scope.node = data;
                $('#editNodeModal').modal().on('hidden.bs.modal', function() {
                    $route.reload();
                });
            }).error(function(data){alert(data.Error)});
        };

        $scope.updateNode = function(node) {
            $http.put('/api/v1/node/' + node.devEUI, node).success(function(data) {
                $('#editNodeModal').modal('hide');
			}).error(function(data) {
                $scope.error = data.Error;
            });
        };

        $scope.deleteNode = function(node) {
            if (confirm('Are you sure you want to delete ' + node.devEUI + '?')) {
                $http.delete('/api/v1/node/' + node.devEUI).success(function(data) {
                    $route.reload();
                }).error(function(data) {alert(data.Error)});
            }
        };

        $scope.editNodeSession = function(node) {
            $http.get('/api/v1/nodeSession/devEUI/' + node.devEUI).success(function(data) {
                $scope.ns = data;
                $('#nodeSessionModal').modal().on('hidden.bs.modal', function() {
                    $route.reload();
                });
            }).error(function(data){
                $scope.ns = {
                    devEUI: node.devEUI,
                    appEUI: node.appEUI,
                    fCntUp: 0,
                    fCntDown: 0
                };
                $('#nodeSessionModal').modal().on('hidden.bs.modal', function() {
                    $route.reload();
                });
			});
        };

        $scope.updateNodeSession = function(ns) {
            $http.put('/api/v1/nodeSession/' + ns.devAddr, ns).success(function(data) {
                $('#nodeSessionModal').modal('hide');
            }).error(function(data) {
                $scope.error = data.Error; 
			});
        };

        $scope.getRandomDevAddr = function(ns) {
            $http.post('/api/v1/nodeSession/getRandomDevAddr').success(function(data) {
                ns.devAddr = data.devAddr;
            }).error(function(data) { 
                $scope.error = data.Error;
			});
        };
    }]);

// manage channel lists
loraserverControllers.controller('ChannelListListCtrl', ['$scope', '$http', '$routeParams', '$route',
    function ($scope, $http, $routeParams, $route) {
        $scope.page = 'channels';
        $http.get('/api/v1/channelList/0/9999').success(function(data) {
            $scope.channelLists = data.result;
        }).error(function(data){alert(data.Error)});

        $scope.createList = function(cl) {
            if (cl == null) {
                $('#createChannelListModal').modal().on('hidden.bs.modal', function() {
                    $route.reload();
                });
            } else {
                $http.post('/api/v1/channelList', cl).success(function(data) {
                    $('#createChannelListModal').modal('hide');
				}).error(function(data) {
                    $scope.error = data.Error;
                });
            }
        };

        $scope.editList = function(cl) {
            $http.get('/api/v1/channelList/' + cl.id).success(function(data) {
                $scope.channelList = data;
                $('#editChannelListModal').modal().on('hidden.bs.modal', function() {
                    $route.reload();
                });
            }).error(function(data){alert(data.Error)});
        };

        $scope.updateList = function(cl) {
            $http.put('/api/v1/channelList/' + cl.id, cl).success(function(data) {
                $('#editChannelListModal').modal('hide');
			}).error(function(data) {
                $scope.error = data.Error;
            });
        };

        $scope.deleteList = function(cl) {
            if (confirm('Are you sure you want to delete ' + cl.name + '?')) {
                $http.delete('/api/v1/channelList/' + cl.id).success(function(data) {
                    $route.reload();
                }).error(function(data){alert(data.Error)});
            }
        };
    }]);

// manage channel list
loraserverControllers.controller('ChannelListCtrl', ['$scope', '$http', '$routeParams', '$route',
    function ($scope, $http, $routeParams, $route) {
        $scope.page = 'channels';
        $http.get('/api/v1/channelList/' + $routeParams.list).success(function(data) {
            $scope.channelList = data;
        }).error(function(data){alert(data.Error)});
        $http.get('/api/v1/channel/channelList/' + $routeParams.list).success(function(data) {
            $scope.channels = data.result;
        }).error(function(data){alert(data.Error)});

        $scope.createChannel = function(c) {
            if (c == null) {
                $('#createChannelModal').modal().on('hidden.bs.modal', function() {
                    $route.reload();
                });
            } else {
                c.channelListID = $scope.channelList.id;
                $http.post('/api/v1/channel', c).success(function(data) {
                    $('#createChannelModal').modal('hide');
				}).error(function(data) {
                    $scope.error = data.Error;
                });
            }
        };

        $scope.editChannel = function(c) {
            $http.get('/api/v1/channel/' + c.id).success(function(data) {
                $scope.channel = data;
                $('#editChannelModal').modal().on('hidden.bs.modal', function() {
                    $route.reload();
                });
            }).error(function(data){alert(data.Error)});
        };

        $scope.updateChannel = function(c) {
            $http.put('/api/v1/channel/' + c.id, c).success(function(data) {
                $('#editChannelModal').modal('hide');
			}).error(function(data) {
                $scope.error = data.Error;
            });
        };

        $scope.deleteChannel = function(c) {
            if (confirm('Are you sure you want to delete channel # ' + c.channel + '?')) {
                $http.delete('/api/v1/channel/' + c.id).success(function(data) {
                    $route.reload();
                }).error(function(data){alert(data.Error)});
            }
        };
    }]);
